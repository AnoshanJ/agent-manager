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

# type -P searches PATH only. `command -v` would also match the docker and k3d
# shell functions defined just below and report them as installed even on a
# host that has neither binary.
have() { type -P "$1" >/dev/null 2>&1; }

# Every docker and k3d call here talks to a daemon that may be wedged — this
# action runs after the job that broke things, so a hung daemon is a normal
# input, not an edge case. An unbounded call would make the cleanup itself the
# thing that needs cleaning up. Wrap them so each one gives up and lets the
# rest of the teardown proceed.
# Resolve the real binaries up front. `timeout` execs a program, so it cannot
# be handed the `command` builtin — and the no-timeout fallback below would
# recurse into these same wrappers if it re-ran the bare name. Absolute paths
# avoid both traps.
DOCKER_BIN="$(type -P docker || true)"
K3D_BIN="$(type -P k3d || true)"

run_bounded() {
  local secs="$1"
  shift
  if have timeout; then
    timeout --kill-after=10s "${secs}" "$@"
  else
    "$@"
  fi
}
[ -n "${DOCKER_BIN}" ] && docker() { run_bounded 180 "${DOCKER_BIN}" "$@"; }
[ -n "${K3D_BIN}" ]    && k3dcmd() { run_bounded 300 "${K3D_BIN}" "$@"; }

echo "::group::Teardown — cluster and workloads"

# --- k3d ------------------------------------------------------------------
# Deleting the cluster is what frees the published host ports (6550, 8080,
# 8443, 10082, 11080/11082/11085, 19080, 19443) the next run's port check
# requires, and it drops the kubeconfig contexts along with the containers.
if have k3d; then
  clusters="$(k3dcmd cluster list --no-headers 2>/dev/null | awk '{print $1}')"
  if [ -n "${clusters}" ]; then
    log "Deleting k3d clusters: $(echo "${clusters}" | tr '\n' ' ')"
    k3dcmd cluster delete --all >/dev/null 2>&1 || warn "k3d cluster delete --all did not complete cleanly; the container sweep below should still remove the nodes."
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

# Skip only the Docker-specific sweeps when there is no daemon here — the
# credential and workspace cleanup further down still has to run, since a job
# can leave a registry token behind without ever touching Docker (helm does
# exactly that).
if ! have docker; then
  log "No Docker on this runner; skipping the container, image and volume sweep"
fi

if have docker; then

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
  run_bounded 600 "${DOCKER_BIN}" system prune -af --volumes >/dev/null 2>&1 || true
  ok "Pruned all unused images, volumes and networks"
else
  docker container prune -f >/dev/null 2>&1 || true
  docker volume prune -f >/dev/null 2>&1 || true
  docker network prune -f >/dev/null 2>&1 || true
  ok "Pruned stopped containers, unused volumes and networks (image cache kept)"
fi

echo "::endgroup::"

fi  # have docker

echo "::group::Teardown — credentials and workspace"

# --- registry credentials --------------------------------------------------
# docker/login-action writes ~/.docker/config.json and `helm registry login`
# inside package-helm-chart.sh writes helm's own store; both persist on a VM
# runner.
if [ -n "${REGISTRY}" ]; then
  docker logout "${REGISTRY}" >/dev/null 2>&1 && log "docker logout ${REGISTRY}" || true
  have helm && helm registry logout "${REGISTRY}" >/dev/null 2>&1 && log "helm registry logout ${REGISTRY}" || true
fi

# Then delete the stores outright, rather than trusting the logouts above.
# `logout <host>` only removes an entry filed under exactly that key, and
# package-helm-chart.sh logs in to "${HELM_REGISTRY#oci://}" — a host *and
# path* — so a logout of the bare host can miss it and report failure, which
# is what happened in run 31617009772: the surviving token made the e2e job's
# anonymous chart pull 403. Removing the file needs no key to match.
for cred in \
  "${DOCKER_CONFIG:-${HOME}/.docker}/config.json" \
  "${HOME}/.docker/config.json" \
  "${HELM_REGISTRY_CONFIG:-${HOME}/.config/helm/registry/config.json}" \
  "${HOME}/.config/helm/registry/config.json"
do
  [ -f "${cred}" ] || continue
  rm -f "${cred}" && log "Removed credential store ${cred}" || true
done

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
