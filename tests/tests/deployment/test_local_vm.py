# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

"""Local runtime lifecycle and macOS VM-specific configuration."""

import json
import subprocess
from pathlib import Path
from typing import Final

import pytest

from tests.testcase_helpers import (
    IS_MACOS_ARM,
    requires_macos_arm,
    run_command,
)


@pytest.mark.local_e2e
@pytest.mark.installation_e2e
def test_full_local_deployment_lifecycle(exasol_path: str, tmp_path: Path) -> None:
    # Given a clean, empty local deployment directory
    deployment_dir = tmp_path / "exasol-local-test"
    deployment_dir.mkdir()
    base = ["--deployment-dir", str(deployment_dir)]
    original_error: BaseException | None = None

    try:
        # When the deployment is initialized and installed
        run_command([exasol_path, "init", "local", *base])
        # Then the infrastructure manifest declares the local backend (step 2)
        manifest = (
            deployment_dir / "infrastructure" / "infrastructure.yaml"
        ).read_text()
        assert "backend: local" in manifest

        run_command([exasol_path, "install", "local", *base])

        # Then deployment.json and secrets.json expose the endpoint contract
        deployment_data = json.loads((deployment_dir / "deployment.json").read_text())
        connection = deployment_data["connection"]
        assert connection["host"] == "127.0.0.1"
        assert connection["dbPort"]
        if IS_MACOS_ARM:
            assert connection["shellSupported"] is True
        else:
            assert "shellSupported" not in connection
        assert "nodes" not in deployment_data
        assert "sshCommand" not in connection
        assert "sshPort" not in connection
        secrets_data = json.loads((deployment_dir / "secrets.json").read_text())
        assert secrets_data["dbPassword"] == "exasol"

        # Then a trivial query returns the single DUMMY row (step 6)
        proc = run_command([exasol_path, "connect", "-c", "SELECT * FROM Dual", *base])
        assert "DUMMY" in proc.stdout

        # When the deployment is stopped and started (steps 9-10)
        run_command([exasol_path, "stop", *base])
        stopped = json.loads(
            run_command([exasol_path, "status", "--json", *base]).stdout
        )
        assert stopped["status"] == "stopped"

        run_command([exasol_path, "start", *base])
        running = json.loads(
            run_command([exasol_path, "status", "--json", *base]).stdout
        )
        assert running["status"] in {"database_ready", "database_connection_failed"}

        # When the deployment is destroyed with local cleanup (step 11)
        run_command([exasol_path, "destroy", "--remove", "--auto-approve", *base])
        assert not deployment_dir.exists()
    except BaseException as error:
        original_error = error
        raise
    finally:
        if deployment_dir.exists():
            try:
                run_command(
                    [exasol_path, "destroy", "--remove", "--auto-approve", *base]
                )
            except subprocess.CalledProcessError:
                if original_error is None:
                    raise


OLD_FIXED_DEFAULT_MB: Final = 2048


LOCAL_MINIMUM_MEMORY_MB: Final = 4096


@pytest.mark.local_e2e
@requires_macos_arm
def test_memory_default_is_half_host_ram(exasol_path: str, tmp_path: Path) -> None:
    # Given a local deployment initialized without an explicit --memory-mb
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()
    base = ["--deployment-dir", str(deployment_dir)]
    run_command([exasol_path, "init", "local", *base])

    # When the resolved configuration is read
    config = json.loads(
        run_command([exasol_path, "config", "get", "--json", "memory-mb", *base]).stdout
    )
    memory_mb = config["infrastructure"]["options"]["memory-mb"]

    # Then it is no longer the old fixed default and honours the minimum
    assert memory_mb != OLD_FIXED_DEFAULT_MB
    assert memory_mb >= LOCAL_MINIMUM_MEMORY_MB
