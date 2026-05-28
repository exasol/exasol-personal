# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

import json
import os
import subprocess
from pathlib import Path

from .helpers import first_infrastructure_preset_id_or_skip


def _env_with_home(home: Path) -> dict[str, str]:
    env = os.environ.copy()
    env["HOME"] = str(home)
    env["USERPROFILE"] = str(home)
    env["HOMEDRIVE"] = ""
    env["HOMEPATH"] = ""
    return env


def _default_dir_logged(stderr: str, default_dir: Path) -> bool:
    for line in stderr.splitlines():
        try:
            decoded: object = json.loads(line)
        except json.JSONDecodeError:
            continue
        if not isinstance(decoded, dict):
            continue
        if decoded.get("msg") != "using default deployment directory":
            continue

        logged_path = decoded.get("path")
        if isinstance(logged_path, str) and Path(logged_path) == default_dir:
            return True

    return False


def _assert_default_dir_logged(stderr: str, default_dir: Path) -> None:
    assert _default_dir_logged(stderr, default_dir), (
        f"expected default deployment directory {str(default_dir)!r} "
        f"in stderr logs:\n{stderr}"
    )


def test_status_uses_default_deployment_dir_without_corrupting_json(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given a home directory without a default deployment directory
    home = tmp_path / "home"
    cwd = tmp_path / "work"
    home.mkdir()
    cwd.mkdir()
    default_dir = home / ".exasol" / "personal" / "deployments" / "default"
    launcher = str(Path(exasol_path).resolve())

    # When status is invoked outside a deployment directory
    result = subprocess.run(
        [launcher, "status", "--json"],
        cwd=cwd,
        env=_env_with_home(home),
        capture_output=True,
        text=True,
        check=True,
    )

    # Then stdout stays parseable JSON and status reports the default directory
    data = json.loads(result.stdout)
    assert data["status"] == "not_initialized"
    assert data["deploymentDir"] == str(default_dir)
    _assert_default_dir_logged(result.stderr, default_dir)


def test_status_reports_uninitialized_explicit_deployment_dir(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given an explicit uninitialized deployment directory
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()
    launcher = str(Path(exasol_path).resolve())

    # When status is invoked for that directory
    result = subprocess.run(
        [launcher, "status", "--json", "--deployment-dir", str(deployment_dir)],
        capture_output=True,
        text=True,
        check=True,
    )

    # Then it succeeds and reports not_initialized
    data = json.loads(result.stdout)
    assert data["status"] == "not_initialized"
    assert data["deploymentDir"] == str(deployment_dir)


def test_init_creates_default_deployment_dir(exasol_path: str, tmp_path: Path) -> None:
    # Given no deployment directory flag and no recognized current deployment directory
    home = tmp_path / "home"
    cwd = tmp_path / "work"
    home.mkdir()
    cwd.mkdir()
    default_dir = home / ".exasol" / "personal" / "deployments" / "default"
    infra_id = first_infrastructure_preset_id_or_skip(exasol_path)
    launcher = str(Path(exasol_path).resolve())

    # When init is invoked
    result = subprocess.run(
        [launcher, "init", infra_id, "--no-launcher-version-check"],
        cwd=cwd,
        env=_env_with_home(home),
        capture_output=True,
        text=True,
        check=True,
    )

    # Then init creates and initializes the default deployment directory
    assert (default_dir / ".exasolLauncherState.json").exists()
    _assert_default_dir_logged(result.stderr, default_dir)


def test_initialized_state_error_mentions_resolved_default_dir(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given no initialized deployment is available in the current or default directory
    home = tmp_path / "home"
    cwd = tmp_path / "work"
    home.mkdir()
    cwd.mkdir()
    default_dir = home / ".exasol" / "personal" / "deployments" / "default"
    launcher = str(Path(exasol_path).resolve())

    # When a command requiring initialized state is invoked
    result = subprocess.run(
        [launcher, "info"],
        cwd=cwd,
        env=_env_with_home(home),
        capture_output=True,
        text=True,
        check=False,
    )

    # Then the error explains the resolved deployment directory
    assert result.returncode != 0
    assert "deployment directory is not initialized" in result.stderr.lower()
    _assert_default_dir_logged(result.stderr, default_dir)
