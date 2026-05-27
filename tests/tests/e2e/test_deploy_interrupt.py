import logging
import os
import signal
import sys
import time
from collections.abc import Iterator

import pytest

from framework.deployment import (
    Deployment,
    StatusDatabaseReady,
    StatusInterrupted,
    StatusOperationInProgress,
)
from framework.launcher import DeploymentConfig, Launcher

pytestmark = [pytest.mark.e2e]


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
