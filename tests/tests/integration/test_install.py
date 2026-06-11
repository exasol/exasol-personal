# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

import json
import os
import platform
import sys
from pathlib import Path
from subprocess import CalledProcessError

import pytest

from .helpers import first_infrastructure_preset_id_or_skip, run_command

LOCAL_TEST_DB_PORT = 28563


def test_install_requires_infra_preset_arg(exasol_path: str) -> None:
    # Given the install command

    # When it is invoked without arguments
    with pytest.raises(CalledProcessError) as exc:
        run_command([exasol_path, "install"])

    # Then it fails because the required infra preset argument is missing
    assert exc.value.returncode != 0
    assert (
        "requires" in (exc.value.stderr or "").lower()
        or "accepts" in (exc.value.stderr or "").lower()
    )


def test_install_help(exasol_path: str) -> None:
    # Given the install command

    # When help is invoked
    result = run_command([exasol_path, "install", "--help"])
    output: str = result.stdout.strip()

    # Then the output explains the command
    assert "Initialize, apply configuration, and deploy Exasol in one step" in output

    # Then I see which preset names I can pass
    infra_id = first_infrastructure_preset_id_or_skip(exasol_path)
    assert "Available infrastructure presets:" in output
    assert infra_id in output
    assert "Available installation presets:" in output
    assert "exasol presets" in output


def test_install_executes_init_step(exasol_path: str, tmp_path: Path) -> None:
    # Given a non-empty deployment directory
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()
    (deployment_dir / "somefile.txt").write_text("x")

    # Given an infrastructure preset ID
    infra_id = first_infrastructure_preset_id_or_skip(exasol_path)

    # When the install command is invoked
    with pytest.raises(CalledProcessError) as excinfo:
        run_command(
            [
                exasol_path,
                "install",
                infra_id,
                "--deployment-dir",
                str(deployment_dir),
            ]
        )

    # Then it fails during initialization (proving init ran)
    assert excinfo.value.returncode != 0
    stderr = (excinfo.value.stderr or "").lower()
    assert "initialization failed" in stderr
    assert "deployment directory is not empty" in stderr


@pytest.mark.skipif(
    sys.platform == "darwin" and platform.machine().lower() in {"arm64", "aarch64"},
    reason="local deployments are supported on macOS Apple Silicon",
)
def test_init_local_rejects_unsupported_platform_before_writing_files(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given an empty deployment directory on an unsupported local platform
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()

    # When init is invoked for the local preset
    with pytest.raises(CalledProcessError) as exc:
        run_command(
            [
                exasol_path,
                "init",
                "local",
                "--deployment-dir",
                str(deployment_dir),
                "--no-launcher-version-check",
            ]
        )

    # Then it fails before writing deployment state
    stderr = exc.value.stderr.lower()
    assert "local deployments are only supported on macos apple silicon" in stderr
    assert list(deployment_dir.iterdir()) == []


@pytest.mark.skipif(
    sys.platform.startswith("win"),
    reason="fake local runner script is POSIX-only",
)
def test_deploy_local_with_prestaged_fake_runner(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given a fake local runner that implements the runner contract without a VM
    runner = tmp_path / "fake-mac-runner-aarch64"
    runner.write_text(
        """#!/usr/bin/env sh
set -eu
case "$1" in
  init)
    mkdir -p vm vm-shared
    ;;
  start)
    if [ "$2" != "2" ] || [ "$3" != "2048" ] || [ "$4" != "100" ]; then
      echo "unexpected sizing args: $*" >&2
      exit 3
    fi
    mkdir -p vm vm-shared
    cat > vm-state.json <<'JSON'
{"vm_name":"exasol-local-vm","vm_ip":"192.168.64.2","ports":{"ssh":20022,"db":28563,"ui":0}}
JSON
    ;;
  stop)
    exit 0
    ;;
  *)
    echo "unexpected command: $1" >&2
    exit 2
    ;;
esac
"""
    )
    runner.chmod(0o700)

    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()
    env = {
        **os.environ,
        "EXASOL_LOCAL_ALLOW_UNSUPPORTED_PLATFORM": "1",
        "EXASOL_LOCAL_SKIP_DB_WAIT": "1",
    }

    # Given a local deployment directory
    init_result = run_command(
        [
            exasol_path,
            "init",
            "local",
            "--deployment-dir",
            str(deployment_dir),
        ],
        env=env,
    )
    assert init_result.returncode == 0

    runner_target = deployment_dir / "local" / "runtime" / "mac-runner-aarch64"
    runner_target.parent.mkdir(parents=True)
    runner_target.write_bytes(runner.read_bytes())
    runner_target.chmod(0o700)

    # When local deploy is invoked
    result = run_command(
        [
            exasol_path,
            "deploy",
            "--deployment-dir",
            str(deployment_dir),
        ],
        env=env,
    )

    # Then the deployment is initialized and local connection artifacts are written
    assert result.returncode == 0
    deployment_data = json.loads((deployment_dir / "deployment.json").read_text())
    assert deployment_data["backend"] == "local"
    assert "nodes" not in deployment_data
    assert deployment_data["connection"]["host"] == "127.0.0.1"
    assert deployment_data["connection"]["dbPort"] == LOCAL_TEST_DB_PORT
    assert "adminUi" not in deployment_data["connection"]
    assert "uiPort" not in deployment_data["connection"]
    assert deployment_data["connection"]["insecureSkipCertValidation"] is True

    secrets_data = json.loads((deployment_dir / "secrets.json").read_text())
    assert secrets_data["dbPassword"] == "exasol"
    assert "adminUiPassword" not in secrets_data

    # Then info can consume the local artifacts without cloud-specific assumptions
    info_result = run_command(
        [
            exasol_path,
            "info",
            "--json",
            "--deployment-dir",
            str(deployment_dir),
        ],
        env=env,
    )
    info_data = json.loads(info_result.stdout)
    assert info_data["backend"] == "local"
    assert info_data["connection"]["dbPort"] == LOCAL_TEST_DB_PORT
    assert "adminUi" not in info_data["connection"]
    assert "uiPort" not in info_data["connection"]

    # Then status also handles the local artifacts even though the fake DB is absent
    status_result = run_command(
        [
            exasol_path,
            "status",
            "--json",
            "--deployment-dir",
            str(deployment_dir),
        ],
        env=env,
    )
    status_data = json.loads(status_result.stdout)
    assert status_data["status"] == "database_connection_failed"

    # Then stop updates the persisted local deployment state
    stop_result = run_command(
        [
            exasol_path,
            "stop",
            "--deployment-dir",
            str(deployment_dir),
        ],
        env=env,
    )
    assert stop_result.returncode == 0
    deployment_data = json.loads((deployment_dir / "deployment.json").read_text())
    assert deployment_data["deploymentState"] == "stopped"
    assert deployment_data["clusterState"] == "stopped"
