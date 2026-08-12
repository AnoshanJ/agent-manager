# Self-hosted runner VM — provisioning notes

`.github/workflows/nightly.yml` runs on a dedicated, long-lived VM instead of GitHub-hosted runners. The workflow provisions its own tooling (this action) and cleans up after itself (`../teardown-self-hosted`), so the VM needs very little set up by hand — but the few things it does need are not optional.

## What the VM must provide

| Requirement | Why |
| --- | --- |
| Ubuntu (or any `apt-get` distro), x86_64 | The pinned k3d/yq checksums cover `linux/amd64` only; the action fails fast on other architectures rather than skipping verification. |
| Passwordless `sudo` for the runner user | Needed once, to install the Docker engine and OS packages and to raise the kernel limits below. Everything else installs into the runner user's home. |
| The runner agent running **directly on the VM** (systemd service), not in a container | See "The #1233 trap" below. |
| Exactly **one** runner registered on the VM | Teardown prunes the whole Docker state; a second concurrent job on the same daemon would have its images and containers removed mid-run. |
| ≥ 4 vCPU, ≥ 16 GiB RAM, ≥ 60 GiB free on `/` | The e2e job stands up OpenChoreo's four planes plus a per-environment Thunder and gateway stack. Below this, pods sit `Pending` on "Insufficient cpu" and k3s starts evicting once imagefs passes 90%. The preflight reports the actual numbers and warns. |
| Outbound HTTPS to ghcr.io, github.com, dl.k8s.io, get.helm.sh, docker.io, quay.io | Tool downloads plus every image the platform install pulls. |

Nothing else: `docker`, `helm`, `kubectl`, `k3d`, `yq`, `gh`, `go`, `jq`, `curl`, `git`, `lsof` and `openssl` are all installed by the workflow on first run and reused thereafter.

## What gets installed where

- Pinned single binaries (k3d, yq, kubectl, helm, gh) → `~/.local/share/amp-ci/bin`, prepended to `PATH` via `$GITHUB_PATH`. Every download is checksum-verified: k3d and yq against digests pinned in `install-tools.sh` (matching `../amp-dev-stack`), kubectl, helm and gh against the digest each project publishes alongside the release.
- Docker engine → system-wide, via the official `get.docker.com` script, only when `docker` is absent. The runner user is added to the `docker` group (durable) *and* granted socket access with `setfacl` (effective immediately, since a group change does not apply to the already-running runner process).
- Go → `actions/setup-go`, into the runner tool cache, unchanged from before.
- Kernel limits → `vm.max_map_count=262144` and raised `fs.inotify.*`, written to `/etc/sysctl.d/99-amp-ci.conf` so a reboot does not regress them. Without the first, the observability plane's OpenSearch fails its bootstrap check and CrashLoopBackOffs with no hint that a host sysctl is responsible.

Bumping a pin in `install-tools.sh` is enough to replace an already-installed binary: each tool is reinstalled when its reported version does not contain the pin.

## The #1233 trap

[PR #1233](https://github.com/wso2/agent-manager/pull/1233) had to move the instrumentation matrix's heavy tier *back* to GitHub-hosted runners because k3d could not work on the CodeBuild self-hosted runners. Those runners reach a Docker-in-Docker daemon, so k3d publishes the API server's host port (`kubeAPI: 6550`) into the *daemon's* network namespace rather than the runner's. The cluster reports "created successfully" and then every `kubectl` call gets connection-refused until the readiness wait times out.

The same failure will hit this VM if the runner is containerised against a shared daemon, or points at a remote `DOCKER_HOST`. `../preflight-k3d` checks for it explicitly before anything expensive runs: it publishes a throwaway container port on `127.0.0.1` and curls it. If that fails the job stops in seconds with an explanation, instead of timing out several minutes into the install.

Run the runner agent directly on the VM against a local Docker Engine and the check passes. If it must stay containerised, give it the daemon's network namespace so published ports and the runner share one.

## Falling back to GitHub-hosted

The nightly's `workflow_dispatch` takes a `runner` input (default `self-hosted`). Dispatch it with `ubuntu-latest` to run everything on GitHub-hosted runners for one run — useful for isolating whether a failure is the VM or the code.

Note that `self-hosted` matches *any* self-hosted runner registered for the repo or org. If more get registered later, give this VM a distinctive label and change the input's default to it.
