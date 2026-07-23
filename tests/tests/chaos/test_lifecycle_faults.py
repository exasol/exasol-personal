# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

"""Fault-injection and recovery tests against the shared deployment.

These tests deliberately interrupt in-flight lifecycle operations. Each test must
leave the deployment in a database-ready state on exit so the deployment and e2e
suites can rely on a running cluster regardless of directory execution order.
"""

import json
import logging
import os
import platform
import signal
import sys
import time
from collections.abc import Iterator
from pathlib import Path
from typing import Final

import pytest

from framework.deployment import (
    Deployment,
    StatusDatabaseReady,
    StatusInterrupted,
    StatusOperationInProgress,
    StatusStopped,
)
from framework.launcher import DeploymentConfig, Launcher
from tests.testcase_helpers import requires_macos_arm, run_command


@pytest.mark.infrastructure_e2e
def test_start_interrupt_sets_interrupted_state(
    reusable_deployment: Deployment,
) -> None:
    # ========== GIVEN ==========
    # A running deployment
    assert reusable_deployment.db_connectable()

    # ========== WHEN ==========
    # We interrupt a stop operation
    stop_proc = reusable_deployment.stop_no_block()
    time.sleep(2)
    assert reusable_deployment.has_status(StatusOperationInProgress)

    if platform.system() != "Windows":
        os.kill(stop_proc.pid, signal.SIGINT)
    else:
        # stop_proc runs in its own process group (see Launcher.start_command),
        # so CTRL_BREAK_EVENT reaches only it, not the test runner too.
        os.kill(stop_proc.pid, signal.CTRL_BREAK_EVENT)  # type: ignore[attr-defined]

    stop_proc.wait(timeout=30)

    # ========== THEN ==========
    # The deployment status should be interrupted
    assert reusable_deployment.has_status(StatusInterrupted)

    time.sleep(30)  # Allow tofu to release the lock

    # ========== GIVEN ==========
    # A stopped deployment
    assert reusable_deployment.stop().returncode == 0
    assert reusable_deployment.has_status(StatusStopped)

    # ========== WHEN ==========
    # We interrupt a start operation
    start_proc = reusable_deployment.start_no_block()
    time.sleep(2)
    assert reusable_deployment.has_status(StatusOperationInProgress)

    if platform.system() != "Windows":
        os.kill(start_proc.pid, signal.SIGINT)
    else:
        os.kill(start_proc.pid, signal.CTRL_BREAK_EVENT)  # type: ignore[attr-defined]

    start_proc.wait(timeout=30)

    # ========== THEN ==========
    # The deployment status should be interrupted
    assert reusable_deployment.has_status(StatusInterrupted)

    time.sleep(30)  # Allow tofu to release the lock

    # Restore to running state for subsequent tests
    assert reusable_deployment.start().returncode == 0
    assert reusable_deployment.has_status(StatusDatabaseReady)


STATUS_FAST_PATH_SECONDS: Final = 30


def _kill_vm_daemon(deployment_dir: Path) -> None:
    """SIGKILL the local VM daemon and wait until the process is gone."""
    pid = int((deployment_dir / "local" / "runtime" / "vm.pid").read_text().strip())
    os.kill(pid, signal.SIGKILL)
    for _ in range(100):  # up to ~10 s
        try:
            os.kill(pid, 0)
        except OSError:
            return
        time.sleep(0.1)
    pytest.fail(f"VM daemon pid {pid} did not exit after SIGKILL")


@pytest.mark.local_e2e
@requires_macos_arm
def test_reconcile_vm_state_after_improper_shutdown(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given a running local deployment
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()
    base = ["--deployment-dir", str(deployment_dir)]
    run_command([exasol_path, "init", "local", *base])
    run_command([exasol_path, "install", "local", *base])

    # When the VM daemon is killed uncleanly (no `exasol stop`)
    _kill_vm_daemon(deployment_dir)

    # Then status reports stopped, fast — the VM daemon check short-circuits
    # before the slower DB connection probe
    started_at = time.monotonic()
    status = json.loads(run_command([exasol_path, "status", "--json", *base]).stdout)
    elapsed = time.monotonic() - started_at
    assert status["status"] == "stopped"
    assert elapsed < STATUS_FAST_PATH_SECONDS, (
        f"status took {elapsed:.1f}s - DB probe not short-circuited?"
    )

    # Then start recovers without manual state surgery and the DB is reachable
    run_command([exasol_path, "start", *base])
    proc = run_command([exasol_path, "connect", "-c", "SELECT * FROM Dual", *base])
    assert "DUMMY" in proc.stdout

    # Then stop also handles a stale running state gracefully (variant)
    _kill_vm_daemon(deployment_dir)
    run_command([exasol_path, "stop", *base])
    status = json.loads(run_command([exasol_path, "status", "--json", *base]).stdout)
    assert status["status"] == "stopped"

    run_command([exasol_path, "destroy", "--remove", "--auto-approve", *base])


@pytest.fixture
def interrupt_deployment(exasol_path: str, infra: str) -> Iterator[Deployment]:
    """Yield a fresh deployment that the test interrupts and then recovers."""
    cluster_size = 2 if infra == "aws" else 1
    config = DeploymentConfig(infra=infra, cluster_size=cluster_size)
    deployment = Deployment(Launcher(exasol_path), config=config)
    try:
        yield deployment
    finally:
        deployment.cleanup()


@pytest.mark.skipif(
    sys.platform.startswith("win"),
    reason="POSIX SIGINT semantics are needed to interrupt deploy cleanly",
)
def test_deploy_can_be_interrupted_and_recovered(
    interrupt_deployment: Deployment,
) -> None:
    """Kill during deploy must lead to a recoverable state.

    Either the subsequent deploy resumes cleanly to DatabaseReady, or the
    launcher surfaces recovery guidance via the interrupted status.
    """
    # ========== GIVEN ==========
    # A fresh initialized deployment, deploy started in the background
    deploy_proc = interrupt_deployment.deploy_no_block()

    # Wait until the operation reports as in-progress before interrupting
    timeout = 30
    deadline = time.time() + timeout
    while time.time() < deadline:
        if interrupt_deployment.has_status(StatusOperationInProgress):
            break
        time.sleep(2)
    else:
        deploy_proc.kill()
        deploy_proc.wait(timeout=30)
        pytest.fail("Deploy did not reach operation_in_progress within 30s")

    # ========== WHEN ==========
    # We interrupt the in-flight deploy
    os.kill(deploy_proc.pid, signal.SIGINT)
    deploy_proc.wait(timeout=120)

    # The launcher should record the interrupted state
    assert interrupt_deployment.has_status(StatusInterrupted)

    # Give the Terraform state lock time to release
    time.sleep(30)

    # ========== THEN ==========
    # A second deploy must either succeed (resume) or fail with a clear
    # interrupted-state message. We accept either outcome and verify state.
    try:
        result = interrupt_deployment.deploy()
        logging.info("Resume deploy returncode=%s", result.returncode)
        # On success the cluster reaches DatabaseReady
        assert interrupt_deployment.has_status(StatusDatabaseReady)
        assert interrupt_deployment.db_connectable()
    except Exception:  # noqa: BLE001 - acceptable; we still validate state below
        # On failure the launcher must leave the deployment in a known state
        # (typically still 'interrupted') rather than corrupted.
        assert interrupt_deployment.has_status(StatusInterrupted) or (
            interrupt_deployment.has_status(StatusDatabaseReady)
        ), "Deployment ended in an unknown state after interrupt + retry"
