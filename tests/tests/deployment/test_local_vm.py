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

from tests.testcase_helpers import (
    IS_MACOS_ARM,
    requires_macos_arm,
    run_command,
    run_in_local_vm,
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


@pytest.mark.local_e2e
@pytest.mark.installation_e2e
@requires_macos_arm
def test_macos_host_data_migration_and_recovery(
    exasol_path: str,
    tmp_path: Path,
) -> None:
    # Given a fresh host-backed deployment with data written from both sides
    deployment_dir = tmp_path / "macos-host-data"
    deployment_dir.mkdir()
    base = ["--deployment-dir", str(deployment_dir)]
    host_exa = deployment_dir / "local" / "runtime" / "exa"
    state_path = deployment_dir / "local" / "runtime" / "vm-state.json"
    destroyed = False

    try:
        run_command([exasol_path, "install", "local", *base])
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
        destroyed = True
        assert not (deployment_dir / "local").exists()
    finally:
        if not destroyed and state_path.exists():
            subprocess.run(
                [exasol_path, "destroy", "--auto-approve", *base],
                capture_output=True,
                text=True,
                check=False,
            )
