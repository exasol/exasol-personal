# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

"""Local runtime lifecycle and macOS VM-specific configuration."""

import json
import os
import shutil
import signal
import subprocess
import time
from pathlib import Path
from typing import Final

import pytest

from framework.deployment import Deployment, DeploymentManager, StatusInitialized
from tests.testcase_helpers import (
    IS_MACOS_ARM,
    requires_macos_arm,
    run_command,
    run_in_local_vm,
)

pytestmark = pytest.mark.openspec("exasol-local-deployment")


@pytest.mark.providers("local")
@pytest.mark.platform("linux-amd64", "macos-arm64", "windows-amd64")
@pytest.mark.smoke
def test_full_local_deployment_lifecycle(local_deployment: Deployment) -> None:
    deployment_dir = Path(local_deployment.deployment_dir.name)

    manifest = (deployment_dir / "infrastructure/infrastructure.yaml").read_text()
    assert "backend: local" in manifest

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

    proc = local_deployment.connect(input="SELECT * FROM Dual", capture_output=True)
    assert "DUMMY" in proc.stdout

    local_deployment.stop()
    assert json.loads(local_deployment.status().stdout)["status"] == "stopped"

    local_deployment.start()
    running = json.loads(local_deployment.status().stdout)
    assert running["status"] in {"database_ready", "database_connection_failed"}

    local_deployment.destroy("--auto-approve")
    assert local_deployment.has_status(StatusInitialized)
    assert local_deployment.has_no_deployment()


OLD_FIXED_DEFAULT_MB: Final = 2048


LOCAL_MINIMUM_MEMORY_MB: Final = 4096

HOST_LAYOUT_MARKER: Final = ".exasol-personal-host-layout"
HOST_DATA_SETUP_SQL: Final = (
    "CREATE SCHEMA HOST_DATA; OPEN SCHEMA HOST_DATA; "
    "CREATE TABLE SMOKE (ID DECIMAL(18,0)); INSERT INTO SMOKE VALUES 424242;"
)


def _run_vm_script(
    exasol_path: str,
    deployment_dir: Path,
    script: str,
) -> subprocess.CompletedProcess[str]:
    return run_in_local_vm(exasol_path, deployment_dir, script)


def _create_legacy_guest_data(
    exasol_path: str,
    deployment_dir: Path,
    host_exa: Path,
    base: list[str],
) -> tuple[Path, Path]:
    host_file = host_exa / "host-visible"
    host_file.write_text("from-host")
    _run_vm_script(
        exasol_path,
        deployment_dir,
        'test "$(cat /mnt/host/exa/host-visible)" = from-host\n'
        "printf from-guest > /mnt/host/exa/guest-visible\n",
    )
    assert (host_exa / "guest-visible").read_text() == "from-guest"

    sparse_path = host_exa / "migration-sparse"
    with sparse_path.open("wb") as sparse_file:
        sparse_file.seek(64 * 1024 * 1024 - 1)
        sparse_file.write(b"x")

    run_command([exasol_path, "connect", "-c", HOST_DATA_SETUP_SQL, *base])
    deployment_data = json.loads((deployment_dir / "deployment.json").read_text())
    container_name = f"exasol-db-{deployment_data['deploymentId']}"
    _run_vm_script(
        exasol_path,
        deployment_dir,
        f"podman pause {container_name}\n"
        "rm -rf /var/lib/exa /var/lib/exa.migrated-backup\n"
        "mkdir -p /var/lib/exa\n"
        "cp -a --sparse=always /mnt/host/exa/. /var/lib/exa/\n"
        f"podman unpause {container_name}\n"
        "sync\n",
    )
    run_command([exasol_path, "stop", *base])

    return host_file, sparse_path


@pytest.mark.providers("local")
@pytest.mark.platform("macos-arm64")
@requires_macos_arm
def test_memory_default_is_half_host_ram(deployment: Deployment) -> None:
    # When the resolved configuration is read
    config = json.loads(
        deployment.launcher.run_command(
            "config",
            deployment.deployment_dir.name,
            "get",
            "--json",
            "memory-mb",
        ).stdout
    )
    memory_mb = config["infrastructure"]["options"]["memory-mb"]

    # Then it is no longer the old fixed default and honours the minimum
    assert memory_mb != OLD_FIXED_DEFAULT_MB
    assert memory_mb >= LOCAL_MINIMUM_MEMORY_MB


@pytest.mark.providers("local")
@pytest.mark.platform("macos-arm64")
@requires_macos_arm
def test_macos_host_data_migration_and_recovery(
    exasol_path: str,
    deployment_manager: DeploymentManager,
) -> None:
    # Given a fresh host-backed deployment with data written from both sides
    deployment = deployment_manager.local()
    deployment_dir = Path(deployment.deployment_dir.name)
    base = ["--deployment-dir", str(deployment_dir)]
    host_exa = deployment_dir / "local" / "runtime" / "exa"
    state_path = deployment_dir / "local" / "runtime" / "vm-state.json"

    try:
        assert (host_exa / HOST_LAYOUT_MARKER).read_text() == "1\n"

        # When the host tree is turned into a legacy guest-disk deployment
        host_file, sparse_path = _create_legacy_guest_data(
            exasol_path, deployment_dir, host_exa, base
        )

        # Then ambiguous source and destination data is refused unchanged
        (host_exa / HOST_LAYOUT_MARKER).unlink()
        conflict = subprocess.run(
            [exasol_path, "start", *base],
            capture_output=True,
            text=True,
            encoding="utf-8",
            check=False,
        )
        assert conflict.returncode != 0
        assert "/var/lib/exa" in conflict.stderr
        assert "/mnt/host/exa" in conflict.stderr
        assert host_file.read_text() == "from-host"

        # When the empty destination condition is restored and start is retried
        shutil.rmtree(host_exa.resolve())
        run_command([exasol_path, "start", *base])

        # Then the full tree was migrated and the guest source became a backup
        persisted = run_command(
            [
                exasol_path,
                "connect",
                "-c",
                "SELECT ID FROM HOST_DATA.SMOKE;",
                *base,
            ]
        )
        assert "424242" in persisted.stdout
        assert host_file.read_text() == "from-host"
        assert (host_exa / "guest-visible").read_text() == "from-guest"
        assert (host_exa / HOST_LAYOUT_MARKER).read_text() == "1\n"
        sparse_stat = sparse_path.stat()
        assert sparse_stat.st_blocks * 512 < sparse_stat.st_size // 2
        _run_vm_script(
            exasol_path,
            deployment_dir,
            "test ! -e /var/lib/exa\ntest -e /var/lib/exa.migrated-backup\n",
        )

        # When Nano is restarted normally and the VM is then killed out of band
        run_command([exasol_path, "stop", *base])
        run_command([exasol_path, "start", *base])
        vm_pid = int(json.loads(state_path.read_text())["pid"])
        os.kill(vm_pid, signal.SIGKILL)
        time.sleep(2)
        run_command([exasol_path, "start", *base])

        # Then both recovery paths reuse the same committed host data
        recovered = run_command(
            [
                exasol_path,
                "connect",
                "-c",
                "SELECT ID FROM HOST_DATA.SMOKE;",
                *base,
            ]
        )
        assert "424242" in recovered.stdout

        # When the deployment is destroyed, both host data and the VM are removed
        run_command([exasol_path, "destroy", "--auto-approve", *base])
        assert not (deployment_dir / "local").exists()
    finally:
        deployment_manager.release(deployment)
