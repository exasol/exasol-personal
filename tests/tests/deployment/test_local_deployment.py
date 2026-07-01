# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

"""Tests specific to local VM deployments."""

import json
import os
import signal
import sys
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
    vm_pid_file = deployment_dir / "local" / "runtime" / "vm.pid"
    pid = int(vm_pid_file.read_text().strip())
    os.kill(pid, signal.SIGKILL)

    assert local_deployment.has_status(StatusStopped)

    start_result = local_deployment.start()
    assert start_result.returncode == 0

    assert local_deployment.has_status(StatusDatabaseReady)
