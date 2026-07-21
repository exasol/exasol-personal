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
LOCAL_MINIMUM_MEMORY_MB = 4096


def assert_lifecycle_json_signal(
    stdout: str, deployment_state: str, *, database_ready: bool
) -> None:
    assert json.loads(stdout) == {
        "deploymentState": deployment_state,
        "databaseReady": database_ready,
    }


def _assert_running_start_guidance(
    exasol_path: str,
    deployment_dir: Path,
    env: dict[str, str],
) -> None:
    start_result = run_command(
        [
            exasol_path,
            "start",
            "--deployment-dir",
            str(deployment_dir),
        ],
        env=env,
    )
    assert start_result.returncode == 0
    assert "deployment state is running, but the database is not ready" in (
        start_result.stderr
    )
    assert "exasol status" in start_result.stderr
    assert "exasol stop" in start_result.stderr
    assert "exasol start" in start_result.stderr


def _assert_already_stopped_guidance(
    exasol_path: str,
    deployment_dir: Path,
    env: dict[str, str],
) -> None:
    second_stop_result = run_command(
        [
            exasol_path,
            "stop",
            "--deployment-dir",
            str(deployment_dir),
        ],
        env=env,
    )
    assert second_stop_result.returncode == 0
    assert "deployment is already stopped" in second_stop_result.stderr


def _assert_local_deployment_artifacts(deployment_dir: Path) -> None:
    version_check_marker = (
        deployment_dir / "local" / "runtime" / "start-version-check-enabled"
    )
    assert version_check_marker.read_text() == "false"

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


def _assert_local_info(
    exasol_path: str,
    deployment_dir: Path,
    env: dict[str, str],
) -> None:
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
    assert info_data["deploymentState"] == "running"
    assert info_data["connection"]["backend"] == "local"
    assert info_data["connection"]["dbPort"] == LOCAL_TEST_DB_PORT
    assert "adminUi" not in info_data["connection"]
    assert "uiPort" not in info_data["connection"]


def _assert_database_connection_failed_status(
    exasol_path: str,
    deployment_dir: Path,
    env: dict[str, str],
) -> None:
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
def test_init_local_accepts_explicit_minimum_memory(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given a local deployment directory on a test-enabled unsupported platform
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()
    env = {
        **os.environ,
        "EXASOL_LOCAL_ALLOW_UNSUPPORTED_PLATFORM": "1",
    }

    # When init is invoked with the minimum supported memory
    result = run_command(
        [
            exasol_path,
            "init",
            "local",
            "--deployment-dir",
            str(deployment_dir),
            "--memory-mb",
            "4096",
        ],
        env=env,
    )

    # Then the deployment is initialized with that value
    assert result.returncode == 0
    config_result = run_command(
        [
            exasol_path,
            "config",
            "get",
            "--json",
            "memory-mb",
            "--deployment-dir",
            str(deployment_dir),
        ],
        env=env,
    )
    config_data = json.loads(config_result.stdout)
    configured_memory_mb = config_data["infrastructure"]["options"]["memory-mb"]
    assert configured_memory_mb == LOCAL_MINIMUM_MEMORY_MB


@pytest.mark.skipif(
    sys.platform.startswith("win"),
    reason="fake local runner script is POSIX-only",
)
def test_init_local_rejects_memory_below_minimum(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given a local deployment directory on a test-enabled unsupported platform
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()
    env = {
        **os.environ,
        "EXASOL_LOCAL_ALLOW_UNSUPPORTED_PLATFORM": "1",
    }

    # When init is invoked below the supported minimum memory
    with pytest.raises(CalledProcessError) as exc:
        run_command(
            [
                exasol_path,
                "init",
                "local",
                "--deployment-dir",
                str(deployment_dir),
                "--memory-mb",
                "4095",
            ],
            env=env,
        )

    # Then the user sees the minimum-memory validation message
    assert (
        "local memory-mb must be at least 4096 mb" in (exc.value.stderr or "").lower()
    )
    # Then validation happened before extraction, leaving the directory empty
    # so a corrected retry is not blocked by leftover preset files.
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
  version)
    # Keep the pre-staged test runner across production runner reconciliation.
    printf 'v999.0.0\n'
    ;;
  init)
    mkdir -p vm vm-shared
    ;;
  start)
    shift
    version_check_enabled=""
    while [ "$#" -gt 3 ]; do
      case "$1" in
        --version-check-enabled=*)
          version_check_enabled="${1#*=}"
          shift
          ;;
        --version-check-enabled)
          version_check_enabled="$2"
          shift 2
          ;;
        --version-check-url|--version-check-identity|--version-check-interval-seconds|--ports)
          shift 2
          ;;
        *)
          echo "unexpected start flag: $1" >&2
          exit 3
          ;;
      esac
    done
    if [ "${version_check_enabled}" != "false" ]; then
      echo "expected disabled version check, got: ${version_check_enabled}" >&2
      exit 3
    fi
    printf '%s' "${version_check_enabled}" > start-version-check-enabled
    if [ "$1" != "2" ] || [ "$2" != "2048" ] || [ "$3" != "100" ]; then
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
            "--no-launcher-version-check",
        ],
        env=env,
    )
    assert init_result.returncode == 0
    config_result = run_command(
        [
            exasol_path,
            "config",
            "get",
            "--json",
            "memory-mb",
            "--deployment-dir",
            str(deployment_dir),
        ],
        env=env,
    )
    expected_memory_mb = json.loads(config_result.stdout)["infrastructure"]["options"][
        "memory-mb"
    ]

    runner_target = deployment_dir / "local" / "runtime" / "mac-runner-aarch64"
    runner_target.parent.mkdir(parents=True)
    runner_target.write_text(
        runner.read_text().replace('"2048"', str(expected_memory_mb))
    )
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
    _assert_local_deployment_artifacts(deployment_dir)

    # Then info can consume the local artifacts without cloud-specific assumptions
    _assert_local_info(exasol_path, deployment_dir, env)

    # Then status also handles the local artifacts even though the fake DB is absent
    _assert_database_connection_failed_status(exasol_path, deployment_dir, env)

    # Then start on a running deployment gives recovery guidance instead of failing
    _assert_running_start_guidance(exasol_path, deployment_dir, env)

    # Then stop updates persisted state and emits a JSON ready signal
    stop_result = run_command(
        [
            exasol_path,
            "stop",
            "--json",
            "--deployment-dir",
            str(deployment_dir),
        ],
        env=env,
    )
    assert stop_result.returncode == 0
    assert_lifecycle_json_signal(stop_result.stdout, "stopped", database_ready=False)
    deployment_data = json.loads((deployment_dir / "deployment.json").read_text())
    assert deployment_data["deploymentState"] == "stopped"
    assert deployment_data["clusterState"] == "stopped"

    # Then stopping an already stopped deployment is an idempotent no-op
    _assert_already_stopped_guidance(exasol_path, deployment_dir, env)

    # Then start emits only the JSON ready signal on stdout
    start_result = run_command(
        [
            exasol_path,
            "start",
            "--json",
            "--deployment-dir",
            str(deployment_dir),
        ],
        env=env,
    )
    assert start_result.returncode == 0
    assert_lifecycle_json_signal(start_result.stdout, "running", database_ready=True)
    assert "Connection Instructions" not in start_result.stdout
