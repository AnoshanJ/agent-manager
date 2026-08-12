#!/usr/bin/env bash
#
# Teardown for the long-lived self-hosted runner VM.
#
# A GitHub-hosted runner is discarded after every job, so the nightly could
# assume a pristine host. A dedicated VM cannot: a k3d cluster, its containers,
# volumes and published host ports all survive the job that created them, and
# the *next* nightly would then either reuse a half-broken cluster
# (quick-start/install.sh is idempotent and adopts an existing `amp-local`
# instead of building it fresh) or fail its port-availability check outright.
#
# So this runs with `if: always()` and returns the VM to "tools installed,
# nothing else" — every command is best-effort and the script always exits 0,
# because a cleanup hiccup must never turn a green nightly red.

set -uo pipefail

PRUNE_IMAGES="${PRUNE_IMAGES:-true}"
REGISTRY="${REGISTRY:-}"
KEEP_CONTAINERS="${KEEP_CONTAINERS:-}"

log()  { echo "  $*"; }
ok()   { echo "✅ $*"; }
warn() { echo "::warning::$*"; }

have() { command -v "$1" >/dev/null 2>&1; }

echo "::group::Teardown — cluster and workloads"

# --- k3d ------------------------------------------------------------------
# Deleting the cluster is what frees the published host ports (6550, 8080,
# 8443, 10082, 11080/11082/11085, 19080, 19443) the next run's port check
# requires, and it drops the kubeconfig contexts along with the containers.
if have k3d; then
  clusters="$(k3d cluster list --no-headers 2>/dev/null | awk '{print $1}')"
  if [ -n "${clusters}" ]; then
    log "Deleting k3d clusters: $(echo "${clusters}" | tr '\n' ' ')"
    k3d cluster delete --all >/dev/null 2>&1 || warn "k3d cluster delete --all did not complete cleanly; the container sweep below should still remove the nodes."
  else
    log "No k3d clusters present"
  fi
else
  log "k3d not installed — nothing to delete"
fi

# --- compose --------------------------------------------------------------
# The nightly's e2e job installs via quick-start (no compose), but jobs that
# reuse this action may have started the deployments/docker-compose.yml stack;
# leaving its postgres volume behind would carry state into the next run.
if have docker && [ -f deployments/docker-compose.yml ]; then
  docker compose -f deployments/docker-compose.yml down -v --remove-orphans >/dev/null 2>&1 \
    && log "Compose stack removed" || true
fi

# --- stray port-forwards ---------------------------------------------------
# deployments/setup/port-forward.sh backgrounds kubectl processes that outlive
# the job and would hold host ports (and log noise) into the next one.
pkill -f 'kubectl.*port-forward' >/dev/null 2>&1 && log "Killed stray kubectl port-forwards" || true

echo "::endgroup::"

if ! have docker; then
  ok "Teardown complete (no Docker on this runner)"
  exit 0
fi

echo "::group::Teardown — containers, volumes, networks"

# --- containers ------------------------------------------------------------
# Force-remove everything left over. The one container that must survive is
# this job's own, in the case where the runner itself is containerised against
# the same daemon: removing that one kills the job mid-teardown.
#
# Finding our own id is the delicate part. /proc/self/mountinfo does carry it,
# in the bind mounts Docker sets up for /etc/hosts and friends — but it also
# carries 64-hex *storage layer* ids from the overlay2 root mount, and which
# kind appears first depends on the storage driver. Taking the first match
# happens to be right on some hosts and silently wrong on others, and "silently
# wrong" here means force-removing the runner.
#
# So collect every candidate and let the daemon adjudicate: a real container id
# resolves through `docker inspect`, an overlay layer id does not. SELF_CID
# short-circuits all of it when the caller already knows the answer.
protected=""
if [ -n "${SELF_CID:-}" ]; then
  protected="$(docker inspect --type=container --format '{{.Id}}' "${SELF_CID}" 2>/dev/null)"
  [ -n "${protected}" ] || warn "SELF_CID='${SELF_CID}' does not resolve to a container on this daemon; ignoring it."
fi
if [ -z "${protected}" ]; then
  for candidate in $( { grep -oE '[0-9a-f]{64}' /proc/self/mountinfo; grep -oE '[0-9a-f]{64}' /proc/self/cgroup; } 2>/dev/null | sort -u); do
    resolved="$(docker inspect --type=container --format '{{.Id}}' "${candidate}" 2>/dev/null)"
    [ -n "${resolved}" ] && protected="${protected} ${resolved}"
  done
fi
if [ -n "${protected}" ]; then
  for p in ${protected}; do
    log "Runner is containerised in ${p:0:12}; excluded from the sweep"
  done
else
  log "Runner is not containerised against this daemon; sweeping every container"
fi

leftover="$(docker ps -aq --no-trunc 2>/dev/null)"
if [ -n "${leftover}" ]; then
  for cid in ${leftover}; do
    case " ${protected} " in
      *" ${cid} "*) continue ;;
    esac
    if [ -n "${KEEP_CONTAINERS}" ]; then
      name="$(docker inspect -f '{{.Name}}' "${cid}" 2>/dev/null | sed 's|^/||')"
      # -- so a keep pattern beginning with a hyphen is a pattern, not a flag.
      if echo "${name}" | grep -qE -- "${KEEP_CONTAINERS}"; then
        log "Keeping container ${name} (matches keep-containers)"
        continue
      fi
    fi
    docker rm -f "${cid}" >/dev/null 2>&1 || true
  done
  ok "Removed leftover containers"
else
  log "No containers to remove"
fi

# --- buildx ----------------------------------------------------------------
# docker/setup-buildx-action creates a builder per job. Its own post-step
# removes it on a clean finish, but a cancelled or killed job leaks both the
# builder and its cache volume.
if docker buildx version >/dev/null 2>&1; then
  # Parse the plain table rather than --format, which older buildx lacks.
  docker buildx ls 2>/dev/null | awk '{gsub(/\*$/, "", $1); print $1}' | grep -E '^builder-' | while read -r b; do
    docker buildx rm -f "${b}" >/dev/null 2>&1 && log "Removed buildx builder ${b}" || true
  done
  docker buildx prune -af >/dev/null 2>&1 && log "Pruned buildx cache" || true
fi

# --- images, volumes, networks --------------------------------------------
if [ "${PRUNE_IMAGES}" = "true" ]; then
  # -a (not just dangling) is deliberate: the nightly's whole point is that the
  # next run installs from scratch, and the dated 0.0.0-dev-* tags would
  # otherwise accumulate a new full image set on disk every single night.
  docker system prune -af --volumes >/dev/null 2>&1 || true
  ok "Pruned all unused images, volumes and networks"
else
  docker container prune -f >/dev/null 2>&1 || true
  docker volume prune -f >/dev/null 2>&1 || true
  docker network prune -f >/dev/null 2>&1 || true
  ok "Pruned stopped containers, unused volumes and networks (image cache kept)"
fi

echo "::endgroup::"

echo "::group::Teardown — credentials and workspace"

# --- registry credentials --------------------------------------------------
# docker/login-action writes ~/.docker/config.json, which persists on a VM
# runner; the same for `helm registry login` inside package-helm-chart.sh.
if [ -n "${REGISTRY}" ]; then
  docker logout "${REGISTRY}" >/dev/null 2>&1 && log "docker logout ${REGISTRY}" || true
  have helm && helm registry logout "${REGISTRY}" >/dev/null 2>&1 && log "helm registry logout ${REGISTRY}" || true
fi

# --- persisted git credential ----------------------------------------------
# actions/checkout writes an authorization header into .git/config unless
# persist-credentials is off. Its post-step removes it on a clean finish, but a
# cancelled or timed-out job never reaches that — and on this VM the workspace
# is still there tomorrow. The jobs that push tags need the credential during
# the run, so scrub it here instead of disabling it for them.
if [ -n "${GITHUB_WORKSPACE:-}" ] && [ -d "${GITHUB_WORKSPACE}/.git" ] && have git; then
  git -C "${GITHUB_WORKSPACE}" config --unset-all 'http.https://github.com/.extraheader' 2>/dev/null \
    && log "Cleared a persisted git credential from the workspace" || true
fi

# --- workspace leftovers ---------------------------------------------------
rm -rf /tmp/pod-logs >/dev/null 2>&1 || true
rm -f "${HOME}/.kube/config.lock" >/dev/null 2>&1 || true

echo "::endgroup::"

echo "::group::Teardown — remaining host state"
df -h / 2>/dev/null
docker system df 2>/dev/null || true
docker ps -a 2>/dev/null || true
echo "::endgroup::"

ok "Teardown complete"
exit 0
