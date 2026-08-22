#!/usr/bin/env bash
# start.sh — one-command entry point for the Agent Manager quick-start dev container.
#
# Downloaded/attached as its own release asset (like deployments/vm/bootstrap.sh),
# so it can be fetched and run directly on the host:
#   curl -fsSL <URL>/start.sh -o start.sh && chmod +x start.sh && ./start.sh
#
# It only checks host prerequisites and launches the dev container; the container
# itself runs install.sh automatically once it starts (see entrypoint.sh).
set -euo pipefail

# Stamped to the release version at build time (see .github/scripts/update-install-helpers.sh
# for the equivalent 0.0.0-dev -> version substitution pattern applied to this file).
DEFAULT_VERSION="0.0.0-dev"
IMAGE="${QUICK_START_IMAGE:-ghcr.io/wso2/amp-quick-start}"
MIN_FREE_DISK_GB=20

log() { printf '\033[0;34m[start]\033[0m %s\n' "$*"; }
warn() { printf '\033[0;33m[start] WARNING:\033[0m %s\n' "$*" >&2; }
die() { printf '\033[0;31m[start] ERROR:\033[0m %s\n' "$*" >&2; exit 1; }

usage() {
  cat <<'EOF'
Usage: ./start.sh [--version vX.Y.Z]

Checks local prerequisites, then launches the Agent Manager quick-start dev
container. Installation runs automatically once the container starts.

  --version   Quick-start image tag to run (default: the version this script
              shipped with, or $QUICK_START_VERSION if set)
EOF
}

VERSION="${QUICK_START_VERSION:-$DEFAULT_VERSION}"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --version) VERSION="${2:?--version requires a value}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown argument: $1 (see --help)" ;;
  esac
done

log "Checking prerequisites..."

command -v docker >/dev/null 2>&1 || \
  die "Docker is required but was not found on PATH. Install Docker before continuing: https://docs.docker.com/get-docker/"

if ! docker info >/dev/null 2>&1; then
  msg="Docker is installed but the daemon is not reachable (docker info failed)."
  if [[ "$(uname -s)" == "Darwin" ]]; then
    msg+=$'\n'"On macOS, start Colima with a dedicated profile:"
    msg+=$'\n'"  colima start --profile agent-manager --vm-type=vz --vz-rosetta --network-address --cpu 4 --memory 8"
  else
    msg+=$'\n'"Make sure the Docker daemon is running (e.g. 'sudo systemctl start docker')."
  fi
  die "$msg"
fi
log "Docker is installed and reachable"

ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64|arm64|aarch64) log "Detected architecture: ${ARCH}" ;;
  *) warn "Unrecognized architecture '${ARCH}' — the quick-start image is published for amd64/arm64 only" ;;
esac

# Check free space on the filesystem backing Docker's data (falls back to /var if
# docker info doesn't expose it, e.g. on some rootless/Colima setups).
DOCKER_ROOT="$(docker info --format '{{.DockerRootDir}}' 2>/dev/null || echo /var)"
if command -v df >/dev/null 2>&1; then
  AVAIL_KB="$(df -Pk "$DOCKER_ROOT" 2>/dev/null | awk 'NR==2 {print $4}')"
  if [[ -n "${AVAIL_KB:-}" ]]; then
    AVAIL_GB=$((AVAIL_KB / 1024 / 1024))
    if (( AVAIL_GB < MIN_FREE_DISK_GB )); then
      warn "Only ~${AVAIL_GB} GB free on ${DOCKER_ROOT} — the full platform install (k3d node images, OpenChoreo, AMP) can need ${MIN_FREE_DISK_GB}+ GB. Installation may fail partway through if space runs out."
    else
      log "Free disk space on ${DOCKER_ROOT}: ~${AVAIL_GB} GB"
    fi
  fi
fi

log "Starting quick-start dev container (${IMAGE}:v${VERSION})"
log "The container will run install.sh automatically once it starts (~15-20 minutes)."
exec docker run --rm -it --name amp-quick-start \
  -v /var/run/docker.sock:/var/run/docker.sock \
  --network=host \
  "${IMAGE}:v${VERSION}"
