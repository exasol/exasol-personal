# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

"""Tests specific to local VM deployments."""

import json
import subprocess
import sys
from collections.abc import Iterator
from pathlib import Path
from typing import Final

import pytest

from framework.deployment import Deployment
from framework.launcher import DeploymentConfig, Launcher


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


@pytest.mark.installation_e2e
@pytest.mark.local_e2e
def test_ports_override_sets_db_port(
    local_ports_deployment: tuple[Deployment, int],
) -> None:
    """--ports db:<port> passes the port through to the local runtime.

    The DB is reachable on the specified port.
    """
    deployment, custom_db_port = local_ports_deployment

    deployment_json = Path(deployment.deployment_dir.name) / "deployment.json"
    info = json.loads(deployment_json.read_text())
    assert info["connection"]["dbPort"] == custom_db_port

    proc = deployment.connect(input="SELECT * FROM Dual", capture_output=True)
    assert "DUMMY" in proc.stdout


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


@pytest.mark.installation_e2e
@pytest.mark.local_e2e
def test_data_persists_across_workload_restart(
    local_ports_deployment: tuple[Deployment, int],
) -> None:
    """The Personal-owned /exa directory survives workload replacement."""
    deployment, _ = local_ports_deployment
    deployment.connect(
        input="""
CREATE SCHEMA local_persistence;
CREATE TABLE local_persistence.rows(value VARCHAR(100));
INSERT INTO local_persistence.rows VALUES ('persisted-row');
""",
        capture_output=True,
    )

    assert deployment.stop().returncode == 0
    assert deployment.start().returncode == 0

    result = deployment.connect(
        input="SELECT value FROM local_persistence.rows",
        capture_output=True,
    )
    assert "persisted-row" in result.stdout


@pytest.mark.skipif(
    sys.platform != "darwin", reason="VM recreation applies only to the macOS adapter"
)
@pytest.mark.installation_e2e
@pytest.mark.local_e2e
def test_data_persists_across_vm_recreation(
    local_ports_deployment: tuple[Deployment, int],
) -> None:
    """Deleting provider-owned VM state does not delete virtiofs database data."""
    deployment, _ = local_ports_deployment
    deployment.connect(
        input="""
CREATE SCHEMA vm_recreation;
CREATE TABLE vm_recreation.rows(value VARCHAR(100));
INSERT INTO vm_recreation.rows VALUES ('survived-vm-recreation');
""",
        capture_output=True,
    )
    assert deployment.stop().returncode == 0

    deployment_root = Path(deployment.deployment_dir.name)
    subprocess.run(
        [
            str(deployment_root / "local" / "provider" / "local-vm"),
            "destroy",
            "--state-dir",
            str(deployment_root / "local" / "runtime"),
        ],
        check=True,
    )

    assert deployment.start().returncode == 0
    result = deployment.connect(
        input="SELECT value FROM vm_recreation.rows",
        capture_output=True,
    )
    assert "survived-vm-recreation" in result.stdout


@pytest.mark.installation_e2e
@pytest.mark.local_e2e
def test_python_slc_runs_on_supported_local_platform(
    local_ports_deployment: tuple[Deployment, int],
) -> None:
    """The platform adapter pulls, mounts, and activates the configured SLC."""
    deployment, _ = local_ports_deployment
    install = deployment.launcher.run_command(
        "slc",
        deployment.deployment_dir.name,
        "install",
        "python3",
        "--auto-approve",
        capture_output=True,
    )
    assert install.returncode == 0

    result = deployment.connect(
        input="""
CREATE PYTHON3 SCALAR SCRIPT local_upper(value VARCHAR(100))
RETURNS VARCHAR(100) AS
def run(ctx):
    return ctx.value.upper()
/
SELECT local_upper('slc-ready');
""",
        capture_output=True,
    )
    assert "SLC-READY" in result.stdout
