"""Runtime-hardening probes that never return secret or host data."""

from __future__ import annotations

import ctypes
import os
import tempfile
from pathlib import Path


PR_GET_NO_NEW_PRIVS = 39


def _proc_status() -> dict[str, str]:
    values: dict[str, str] = {}
    try:
        for line in Path("/proc/self/status").read_text(encoding="utf-8").splitlines():
            key, separator, value = line.partition(":")
            if separator:
                values[key] = value.strip()
    except OSError:
        return {}
    return values


def _read_no_new_privileges() -> int | None:
    """Read the process flag through prctl, independent of procfs rendering."""

    try:
        libc = ctypes.CDLL(None, use_errno=True)
        prctl = libc.prctl
    except (AttributeError, OSError):
        return None

    prctl.restype = ctypes.c_int
    return int(
        prctl(
            ctypes.c_int(PR_GET_NO_NEW_PRIVS),
            ctypes.c_ulong(0),
            ctypes.c_ulong(0),
            ctypes.c_ulong(0),
            ctypes.c_ulong(0),
        )
    )


def classify_no_new_privileges(
    prctl_result: int | None,
    proc_value: str | None,
) -> tuple[bool, str]:
    """Classify authoritative prctl evidence without failing open.

    Older gVisor releases maintain the flag but omit NoNewPrivs from procfs.
    Procfs is therefore diagnostic only when prctl itself is unavailable: an
    unsupported direct query must never become a security pass.
    """

    if prctl_result == 1:
        return True, "prctl_enabled"
    if prctl_result == 0:
        return False, "prctl_disabled"
    if proc_value == "1":
        return False, "prctl_unavailable_proc_enabled"
    if proc_value == "0":
        return False, "prctl_unavailable_proc_disabled"
    return False, "prctl_unavailable_proc_missing"


SECCOMP_MODE_FILTER = "2"


def classify_syscall_confinement(
    seccomp_value: str | None,
    kernel_release: str | None,
) -> tuple[bool, str]:
    """Classify the syscall boundary confining this workload, fail-closed.

    Two different mechanisms satisfy the same requirement, and only one of them
    is legible from inside the container:

    - Under runc, the kubelet installs the RuntimeDefault seccomp filter on the
      process itself, so procfs reports Seccomp: 2 and that is authoritative.
    - Under gVisor, the syscall boundary IS the sandbox: the application talks
      to the Sentry, never the host kernel, and the host-side seccomp filter
      confines runsc rather than this process. gVisor's own procfs consequently
      reports Seccomp: 0 (and omits NoNewPrivs), so reading it here says nothing
      about whether the pod is confined.

    Neither reading is allowed to fail open: an unrecognised kernel with no
    seccomp filter is an unconfined workload, not an unsupported probe.
    """

    if seccomp_value == SECCOMP_MODE_FILTER:
        return True, "seccomp_filter"
    if kernel_release and "gvisor" in kernel_release.lower():
        return True, "gvisor_sandbox"
    if seccomp_value in (None, ""):
        return False, "unconfined_proc_missing"
    return False, f"unconfined_seccomp_mode_{seccomp_value}"


def runtime_posture() -> dict[str, object]:
    """Return booleans describing the sandbox without exposing its contents."""

    root_filesystem_read_only = False
    # This directory is owned by the probe's non-root UID in the image. A write
    # therefore succeeds on a writable root filesystem and fails only when the
    # workload's root filesystem is actually mounted read-only.
    root_probe = Path("/security-probe-fs-test/write-check")
    try:
        root_probe.write_text("probe", encoding="utf-8")
        root_probe.unlink(missing_ok=True)
    except OSError:
        root_filesystem_read_only = True

    tmp_writable = False
    try:
        with tempfile.NamedTemporaryFile(prefix="amp-security-probe-", dir="/tmp"):
            tmp_writable = True
    except OSError:
        pass

    status = _proc_status()
    cap_eff = status.get("CapEff", "")
    seccomp = status.get("Seccomp", "")
    no_new_privileges, no_new_privileges_evidence = classify_no_new_privileges(
        _read_no_new_privileges(),
        status.get("NoNewPrivs"),
    )
    syscall_confined, syscall_confinement_evidence = classify_syscall_confinement(
        seccomp,
        os.uname().release,
    )

    return {
        "non_root": os.geteuid() != 0,
        "root_filesystem_read_only": root_filesystem_read_only,
        "tmp_writable": tmp_writable,
        "service_account_token_present": Path(
            "/var/run/secrets/kubernetes.io/serviceaccount/token"
        ).exists(),
        "effective_capabilities_dropped": bool(cap_eff)
        and int(cap_eff, 16) == 0,
        "no_new_privileges": no_new_privileges,
        "no_new_privileges_evidence": no_new_privileges_evidence,
        "seccomp_enabled": seccomp == SECCOMP_MODE_FILTER,
        "syscall_confined": syscall_confined,
        "syscall_confinement_evidence": syscall_confinement_evidence,
    }
