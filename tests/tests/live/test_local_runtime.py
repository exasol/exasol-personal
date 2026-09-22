# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

"""Local runtime lifecycle and macOS VM-specific configuration."""

import json
import os
import platform
import shutil
import signal
import subprocess
import sys
import time
from pathlib import Path
from typing import Final

import pytest

from framework.deployment import Deployment, DeploymentManager, StatusInitialized
from tests.testcase_helpers import (
    IS_MACOS_ARM,
    assert_lifecycle_json_signal,
    run_command,
    run_in_local_vm,
)

pytestmark = pytest.mark.openspec("exasol-local-deployment")

IS_WINDOWS_AMD64: Final = sys.platform.startswith(
    "win"
) and platform.machine().lower() in {
    "amd64",
    "x86_64",
}


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


def _windows_podman_path() -> Path:
    return Path(os.environ["PROGRAMFILES"]) / "RedHat" / "Podman" / "podman.exe"


def _podman_query(podman_path: Path, *args: str) -> str:
    return subprocess.run(
        [str(podman_path), *args],
        capture_output=True,
        text=True,
        check=True,
    ).stdout


def _run_podman(podman_path: Path, *args: str) -> None:
    subprocess.run(
        [str(podman_path), *args],
        capture_output=True,
        text=True,
        check=True,
    )


def _windows_machine_state(podman_path: Path) -> str:
    return (
        _podman_query(
            podman_path,
            "machine",
            "inspect",
            "--format",
            "{{.State}}",
            "podman-machine-default",
        )
        .strip()
        .lower()
    )


def _windows_container_names(podman_path: Path) -> list[str]:
    return _podman_query(podman_path, "ps", "-a", "--format", "{{.Names}}").splitlines()


def _assert_windows_host_data_passthrough(
    podman_path: Path,
    container_name: str,
    host_exa: Path,
    tmp_path: Path,
) -> None:
    mounts = json.loads(
        _podman_query(
            podman_path,
            "inspect",
            "--format",
            "{{json .Mounts}}",
            container_name,
        )
    )
    exa_mounts = [mount for mount in mounts if mount["Destination"] == "/exa"]
    assert len(exa_mounts) == 1
    assert exa_mounts[0]["Type"] == "bind"
    assert host_exa.is_dir()

    host_file = host_exa / "host-visible"
    host_file.write_text("from-host")
    host_roundtrip = tmp_path / "host-roundtrip"
    _run_podman(
        podman_path,
        "cp",
        f"{container_name}:/exa/host-visible",
        str(host_roundtrip),
    )
    assert host_roundtrip.read_text() == "from-host"

    guest_source = tmp_path / "guest-source"
    guest_source.write_text("from-guest")
    _run_podman(
        podman_path,
        "cp",
        str(guest_source),
        f"{container_name}:/exa/guest-visible",
    )
    assert (host_exa / "guest-visible").read_text() == "from-guest"


@pytest.mark.providers("local")
@pytest.mark.platform("windows-amd64")
@pytest.mark.openspec("windows-host-runtime-environment")
@pytest.mark.smoke
@pytest.mark.skipif(
    not IS_WINDOWS_AMD64,
    reason="the real Windows host lifecycle runs only on Windows amd64",
)
def test_install_local_windows_lifecycle(
    deployment_manager: DeploymentManager,
    exasol_path: str,
    tmp_path: Path,
) -> None:
    destroyed = False

    def cleanup(deployment_dir: Path) -> None:
        if not destroyed and (deployment_dir / ".exasolLauncherState.json").exists():
            run_command(
                [
                    exasol_path,
                    "destroy",
                    "--deployment-dir",
                    str(deployment_dir),
                    "--auto-approve",
                    "--verbose",
                ]
            )

    deployment = deployment_manager.local_install_target(cleanup)
    deployment_dir = Path(deployment.deployment_dir.name)
    original_error: BaseException | None = None

    assert shutil.which("podman") is None

    try:
        install_result = run_command(
            [
                exasol_path,
                "install",
                "local",
                "--deployment-dir",
                str(deployment_dir),
                "--auto-approve",
                "--no-launcher-version-check",
                "--verbose",
            ]
        )
        assert install_result.returncode == 0

        podman_path = _windows_podman_path()
        assert podman_path.is_file()
        assert _windows_machine_state(podman_path) == "running"

        deployment_data = json.loads((deployment_dir / "deployment.json").read_text())
        container_name = f"exasol-db-{deployment_data['deploymentId']}"
        host_exa = deployment_dir / "local" / "runtime" / "exa"
        _assert_windows_host_data_passthrough(
            podman_path, container_name, host_exa, tmp_path
        )

        run_command(
            [
                exasol_path,
                "connect",
                "--deployment-dir",
                str(deployment_dir),
                "--command",
                (
                    "CREATE SCHEMA WINDOWS_LIFECYCLE; "
                    "OPEN SCHEMA WINDOWS_LIFECYCLE; "
                    "CREATE TABLE SMOKE (ID DECIMAL(18,0)); "
                    "INSERT INTO SMOKE VALUES 424242;"
                ),
            ]
        )

        stop_result = run_command(
            [
                exasol_path,
                "stop",
                "--json",
                "--deployment-dir",
                str(deployment_dir),
            ]
        )
        assert_lifecycle_json_signal(
            stop_result.stdout, "stopped", database_ready=False
        )
        assert container_name not in _windows_container_names(podman_path)

        start_result = run_command(
            [
                exasol_path,
                "start",
                "--json",
                "--deployment-dir",
                str(deployment_dir),
                "--auto-approve",
            ]
        )
        assert_lifecycle_json_signal(
            start_result.stdout, "running", database_ready=True
        )

        persisted = run_command(
            [
                exasol_path,
                "connect",
                "--csv",
                "--deployment-dir",
                str(deployment_dir),
                "--command",
                "SELECT ID FROM WINDOWS_LIFECYCLE.SMOKE;",
            ]
        )
        assert "424242" in persisted.stdout

        subprocess.run(
            [str(podman_path), "machine", "stop"],
            capture_output=True,
            text=True,
            check=True,
        )
        recovered = run_command(
            [
                exasol_path,
                "start",
                "--json",
                "--deployment-dir",
                str(deployment_dir),
                "--auto-approve",
            ]
        )
        assert_lifecycle_json_signal(recovered.stdout, "running", database_ready=True)

        survived = run_command(
            [
                exasol_path,
                "connect",
                "--csv",
                "--deployment-dir",
                str(deployment_dir),
                "--command",
                "SELECT ID FROM WINDOWS_LIFECYCLE.SMOKE;",
            ]
        )
        assert "424242" in survived.stdout

        run_command(
            [
                exasol_path,
                "destroy",
                "--deployment-dir",
                str(deployment_dir),
                "--auto-approve",
                "--verbose",
            ]
        )
        destroyed = True

        assert container_name not in _windows_container_names(podman_path)
        assert not (deployment_dir / "local").exists()
        assert not host_exa.exists()
        assert _windows_machine_state(podman_path) == "running"
    except BaseException as error:
        original_error = error
        raise
    finally:
        try:
            deployment_manager.release(deployment)
        except RuntimeError:
            if original_error is None:
                raise


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
