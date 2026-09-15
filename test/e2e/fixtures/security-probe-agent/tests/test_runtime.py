"""Tests for fail-closed runtime-hardening evidence classification."""

import unittest

from probe.runtime import classify_no_new_privileges, classify_syscall_confinement


class NoNewPrivilegesClassificationTests(unittest.TestCase):
    def test_prctl_enabled_is_authoritative(self) -> None:
        self.assertEqual(
            classify_no_new_privileges(1, "0"),
            (True, "prctl_enabled"),
        )

    def test_prctl_disabled_fails(self) -> None:
        self.assertEqual(
            classify_no_new_privileges(0, "1"),
            (False, "prctl_disabled"),
        )

    def test_proc_enabled_is_only_diagnostic_when_prctl_is_unavailable(self) -> None:
        self.assertEqual(
            classify_no_new_privileges(None, "1"),
            (False, "prctl_unavailable_proc_enabled"),
        )

    def test_missing_prctl_and_proc_evidence_fails_closed(self) -> None:
        self.assertEqual(
            classify_no_new_privileges(None, None),
            (False, "prctl_unavailable_proc_missing"),
        )


class SyscallConfinementClassificationTests(unittest.TestCase):
    def test_seccomp_filter_is_authoritative_under_runc(self) -> None:
        self.assertEqual(
            classify_syscall_confinement("2", "5.15.0-generic"),
            (True, "seccomp_filter"),
        )

    def test_gvisor_sandbox_is_the_syscall_boundary(self) -> None:
        # gVisor reports Seccomp: 0 to the workload even when the pod carries
        # seccompProfile: RuntimeDefault; the sandbox itself is the boundary.
        self.assertEqual(
            classify_syscall_confinement("0", "4.19.0-gvisor"),
            (True, "gvisor_sandbox"),
        )

    def test_seccomp_filter_wins_over_kernel_sniffing(self) -> None:
        self.assertEqual(
            classify_syscall_confinement("2", "4.19.0-gvisor"),
            (True, "seccomp_filter"),
        )

    def test_unconfined_runc_workload_fails(self) -> None:
        self.assertEqual(
            classify_syscall_confinement("0", "5.15.0-generic"),
            (False, "unconfined_seccomp_mode_0"),
        )

    def test_missing_procfs_evidence_fails_closed(self) -> None:
        self.assertEqual(
            classify_syscall_confinement(None, "5.15.0-generic"),
            (False, "unconfined_proc_missing"),
        )

    def test_unknown_kernel_without_a_filter_never_fails_open(self) -> None:
        self.assertEqual(
            classify_syscall_confinement("", None),
            (False, "unconfined_proc_missing"),
        )


if __name__ == "__main__":
    unittest.main()
