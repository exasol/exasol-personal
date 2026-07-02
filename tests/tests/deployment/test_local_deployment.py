# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

"""Tests specific to local VM deployments."""

import json
import logging
import os
import shutil
import signal
import subprocess
import sys
import time
from collections.abc import Iterator
from pathlib import Path
from typing import Final

import pytest

from framework.deployment import Deployment, StatusDatabaseReady, StatusStopped
from framework.launcher import DeploymentConfig, Launcher


@pytest.fixture
def local_deployment(
    exasol_path: str,
    infra: str,
) -> Iterator[Deployment]:
    if infra != "local":
        pytest.skip("test is local-only")

    config = DeploymentConfig(infra="local")
    deployment = Deployment(Launcher(exasol_path), config=config)
    try:
        deployment.deploy()
        yield deployment
    finally:
        deployment.cleanup()


@pytest.fixture
def local_ports_deployment(
    exasol_path: str,
    infra: str,
) -> Iterator[tuple[Deployment, int]]:
    if infra != "local":
        pytest.skip("ports override is local-only")

    custom_db_port: Final = 9564
    config = DeploymentConfig(infra="local")

    deployment = Deployment(
        Launcher(exasol_path),
        "--ports",
        f"db:{custom_db_port}",
        config=config,
    )
    try:
        deployment.deploy()
        yield deployment, custom_db_port
    finally:
        deployment.cleanup()


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
@pytest.mark.installation_e2e
@pytest.mark.local_e2e
def test_ports_override_sets_db_port(
    local_ports_deployment: tuple[Deployment, int],
) -> None:
    """--ports db:<port> passes the port through to the VM runner.

    The DB is reachable on the specified port.
    """
    deployment, custom_db_port = local_ports_deployment

    deployment_json = Path(deployment.deployment_dir.name) / "deployment.json"
    info = json.loads(deployment_json.read_text())
    assert info["connection"]["dbPort"] == custom_db_port

    proc = deployment.connect(input="SELECT * FROM Dual", capture_output=True)
    assert "DUMMY" in proc.stdout


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
@pytest.mark.installation_e2e
@pytest.mark.local_e2e
def test_ports_override_stable_across_restarts(
    local_ports_deployment: tuple[Deployment, int],
) -> None:
    """Port assignments from --ports db:<port> survive a stop/start cycle.

    The custom DB port must remain unchanged in deployment.json and the DB
    must be reachable on that port after the VM is restarted.
    """
    deployment, custom_db_port = local_ports_deployment

    deployment_json = Path(deployment.deployment_dir.name) / "deployment.json"

    stop_result = deployment.stop()
    assert stop_result.returncode == 0

    info = json.loads(deployment_json.read_text())
    assert info["connection"]["dbPort"] == custom_db_port

    start_result = deployment.start()
    assert start_result.returncode == 0

    info = json.loads(deployment_json.read_text())
    assert info["connection"]["dbPort"] == custom_db_port

    proc = deployment.connect(input="SELECT * FROM Dual", capture_output=True)
    assert "DUMMY" in proc.stdout


def _wait_for_process_exit(
    pid: int, timeout: float = 30, interval: float = 0.2
) -> None:
    """Block until `pid` no longer exists, or raise on timeout."""
    deadline = time.time() + timeout
    while time.time() < deadline:
        try:
            os.kill(pid, 0)
        except ProcessLookupError:
            return
        time.sleep(interval)
    msg = f"process {pid} still alive after {timeout}s"
    raise TimeoutError(msg)


START_TIMEOUT_SECONDS: Final = 180

# Give the freshly deployed DB a moment to settle before we forcefully kill
# the VM daemon, so we're not killing it mid-startup.
SETTLE_BEFORE_KILL_SECONDS: Final = 10

# Populated only when the CI workflow opts into debug artifacts (DEBUG_ARTIFACTS=true).
# If it doesn't exist, no failure copying happens, so normal runs pay no extra cost.
FAILURES_DIR: Final = Path("failures")


_FAILURE_ARTIFACT_FILENAMES: Final = (
    "vm-console.log",
    "vm.log",
    "deployment.json",
    "deployment.log",
)


def _save_failure_artifacts(deployment_dir: Path, test_name: str) -> None:
    if not FAILURES_DIR.is_dir():
        return

    dest = FAILURES_DIR / test_name
    dest.mkdir(parents=True, exist_ok=True)

    for filename in _FAILURE_ARTIFACT_FILENAMES:
        matches = list(deployment_dir.rglob(filename))
        if not matches:
            continue
        source = matches[0]
        logging.info("Test failed. Copying %s to %s for debugging", source, dest)
        shutil.copy2(source, dest / filename)


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
@pytest.mark.installation_e2e
@pytest.mark.local_e2e
def test_status_after_forceful_vm_kill(
    local_deployment: Deployment,
) -> None:
    """Killing the VM daemon with SIGKILL surfaces as stopped status.

    After an unclean shutdown, `status` must report `stopped` rather than
    `database_connection_failed`, and `start` must recover the deployment.
    """
    deployment_dir = Path(local_deployment.deployment_dir.name)
    try:
        logging.info(
            "Letting the database settle for %ds before killing the VM daemon",
            SETTLE_BEFORE_KILL_SECONDS,
        )
        time.sleep(SETTLE_BEFORE_KILL_SECONDS)

        vm_pid_file = deployment_dir / "local" / "runtime" / "vm.pid"
        pid = int(vm_pid_file.read_text().strip())
        logging.info("Sending SIGKILL to VM daemon (pid=%d)", pid)
        os.kill(pid, signal.SIGKILL)
        _wait_for_process_exit(pid)
        logging.info("VM daemon (pid=%d) confirmed stopped", pid)

        logging.info("Waiting for status to report stopped after forceful kill")
        assert local_deployment.has_status(StatusStopped)

        logging.info("Starting deployment to recover from unclean shutdown")
        try:
            start_result = local_deployment.start(timeout=START_TIMEOUT_SECONDS)
        except subprocess.TimeoutExpired:
            msg = f"start did not complete within {START_TIMEOUT_SECONDS}s after kill"
            pytest.fail(msg)
        logging.info(
            "start returned rc=%d, stdout=%s, stderr=%s",
            start_result.returncode,
            start_result.stdout,
            start_result.stderr,
        )
        assert start_result.returncode == 0

        logging.info("Waiting for status to report database ready after recovery")
        assert local_deployment.has_status(StatusDatabaseReady)
    except BaseException:
        _save_failure_artifacts(deployment_dir, "test_status_after_forceful_vm_kill")
        raise
