# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

import json
import platform
import subprocess
import sys
from pathlib import Path

import pytest

from .helpers import record_running_local_deployment, run_command

pytestmark = [
    pytest.mark.openspec("docker-host-runtime"),
    pytest.mark.skipif(
        sys.platform == "darwin"
        or platform.machine().lower()
        not in {
            "amd64",
            "x86_64",
            "arm64",
            "aarch64",
        },
        reason="the container engine is chosen only on Linux and Windows",
    ),
]


def _init(exasol_path: str, deployment_dir: Path, *extra: str) -> None:
    run_command(
        [
            exasol_path,
            "init",
            "local",
            *extra,
            "--deployment-dir",
            str(deployment_dir),
            "--no-launcher-version-check",
        ]
    )


def _configured_engine(exasol_path: str, deployment_dir: Path) -> str:
    result = run_command(
        [
            exasol_path,
            "config",
            "get",
            "--json",
            "container-engine",
            "--deployment-dir",
            str(deployment_dir),
        ]
    )
    return str(
        json.loads(result.stdout)["infrastructure"]["options"]["container-engine"]
    )


def _config_set(
    exasol_path: str, deployment_dir: Path, engine: str
) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        [
            exasol_path,
            "config",
            "set",
            "--container-engine",
            engine,
            "--deployment-dir",
            str(deployment_dir),
        ],
        text=True,
        capture_output=True,
        check=False,
    )


def test_container_engine_defaults_to_auto(exasol_path: str, tmp_path: Path) -> None:
    # Given
    deployment_dir = tmp_path / "deployment"
    _init(exasol_path, deployment_dir)
    # Then
    assert _configured_engine(exasol_path, deployment_dir) == "auto"


def test_container_engine_is_stored_from_init(exasol_path: str, tmp_path: Path) -> None:
    # Given
    deployment_dir = tmp_path / "deployment"
    # When
    _init(exasol_path, deployment_dir, "--container-engine", "docker")
    # Then
    assert _configured_engine(exasol_path, deployment_dir) == "docker"


def test_container_engine_is_set_before_the_first_start(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given
    deployment_dir = tmp_path / "deployment"
    _init(exasol_path, deployment_dir)
    # When
    result = _config_set(exasol_path, deployment_dir, "docker")
    # Then
    assert result.returncode == 0, result.stderr
    assert _configured_engine(exasol_path, deployment_dir) == "docker"


@pytest.mark.parametrize("engine", ["containerd", ""])
def test_unsupported_container_engine_is_rejected(
    exasol_path: str, tmp_path: Path, engine: str
) -> None:
    # Given
    deployment_dir = tmp_path / "deployment"
    _init(exasol_path, deployment_dir, "--container-engine", "docker")
    # When
    result = _config_set(exasol_path, deployment_dir, engine)
    # Then
    assert result.returncode != 0
    assert "must be one of auto, podman, docker" in result.stderr
    assert _configured_engine(exasol_path, deployment_dir) == "docker"


def test_started_deployment_keeps_its_container_engine(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given a deployment whose first start recorded Docker
    deployment_dir = tmp_path / "deployment"
    _init(exasol_path, deployment_dir, "--container-engine", "docker")
    runtime_dir = deployment_dir / "local" / "runtime"
    runtime_dir.mkdir(parents=True)
    (runtime_dir / "container-engine.json").write_text('{"engine":"docker"}')
    # When
    refused = _config_set(exasol_path, deployment_dir, "podman")
    accepted = _config_set(exasol_path, deployment_dir, "auto")
    # Then
    assert refused.returncode != 0
    assert "run `exasol destroy` before changing its container engine" in refused.stderr
    assert "deletes the database and its data" in refused.stderr
    assert accepted.returncode == 0, accepted.stderr
    assert _configured_engine(exasol_path, deployment_dir) == "auto"


def test_running_deployment_names_destroy_for_an_engine_change(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given a running deployment whose first start recorded Docker
    deployment_dir = tmp_path / "deployment"
    _init(exasol_path, deployment_dir, "--container-engine", "docker")
    runtime_dir = deployment_dir / "local" / "runtime"
    runtime_dir.mkdir(parents=True)
    (runtime_dir / "container-engine.json").write_text('{"engine":"docker"}')
    record_running_local_deployment(deployment_dir, 8563)
    # When
    refused = _config_set(exasol_path, deployment_dir, "podman")
    # Then the first refusal already names the step that changes the engine
    assert refused.returncode != 0
    assert "run `exasol destroy` before changing its container engine" in refused.stderr
    assert "exasol stop" not in refused.stderr
