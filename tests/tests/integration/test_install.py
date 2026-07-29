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

LOCAL_MINIMUM_MEMORY_MB = 4096


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
    args = [
        exasol_path,
        "install",
        infra_id,
        "--deployment-dir",
        str(deployment_dir),
    ]
    with pytest.raises(CalledProcessError) as excinfo:
        run_command(args)

    # Then it fails during initialization (proving init ran)
    assert excinfo.value.returncode != 0
    stderr = (excinfo.value.stderr or "").lower()
    assert "initialization failed" in stderr
    assert "deployment directory is not empty" in stderr


@pytest.mark.skipif(
    (
        sys.platform == "darwin"
        and platform.machine().lower() in {"arm64", "aarch64"}
    )
    or (
        sys.platform.startswith("win")
        and platform.machine().lower() in {"amd64", "x86_64"}
    ),
    reason="local deployments are supported on this platform",
)
def test_init_local_rejects_unsupported_platform_before_writing_files(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given an empty deployment directory on an unsupported local platform
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()

    # When init is invoked for the local preset
    args = [
        exasol_path,
        "init",
        "local",
        "--deployment-dir",
        str(deployment_dir),
        "--no-launcher-version-check",
    ]
    with pytest.raises(CalledProcessError) as exc:
        run_command(args)

    # Then it fails before writing deployment state
    stderr = exc.value.stderr.lower()
    assert (
        "local deployments require macos apple silicon or windows amd64 with wsl2"
        in stderr
    )
    assert list(deployment_dir.iterdir()) == []


@pytest.mark.skipif(
    sys.platform.startswith("win"),
    reason="macOS VM resource settings are not exposed on Windows",
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
    reason="macOS VM resource settings are not exposed on Windows",
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
    args = [
        exasol_path,
        "init",
        "local",
        "--deployment-dir",
        str(deployment_dir),
        "--memory-mb",
        "4095",
    ]
    with pytest.raises(CalledProcessError) as exc:
        run_command(args, env=env)

    # Then the user sees the minimum-memory validation message
    assert (
        "local memory-mb must be at least 4096 mb" in (exc.value.stderr or "").lower()
    )
    # Then validation happened before extraction, leaving the directory empty
    # so a corrected retry is not blocked by leftover preset files.
    assert list(deployment_dir.iterdir()) == []
