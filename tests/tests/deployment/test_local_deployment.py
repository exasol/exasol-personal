# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

"""Tests specific to local VM deployments."""

import json
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


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
@pytest.mark.installation_e2e
@pytest.mark.local_e2e
def test_ports_override_sets_db_port(
    local_ports_deployment: tuple[Deployment, int],
) -> None:
    """--ports db:<port> passes the port through to the VM runner and the DB is reachable on it."""
    deployment, custom_db_port = local_ports_deployment

    deployment_json = Path(deployment.deployment_dir.name) / "deployment.json"
    info = json.loads(deployment_json.read_text())
    assert info["connection"]["dbPort"] == custom_db_port

    proc = deployment.connect(input="SELECT * FROM Dual", capture_output=True)
    assert "DUMMY" in proc.stdout
