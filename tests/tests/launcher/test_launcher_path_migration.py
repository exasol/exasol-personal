# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

import json
import subprocess
import sys
from pathlib import Path

import pytest

from .helpers import deployments_root, env_with_home


def _seed_legacy_deployment(home: Path, name: str) -> Path:
    legacy_dir = home / ".exasol" / "personal" / "deployments" / name
    legacy_dir.mkdir(parents=True)
    (legacy_dir / "marker.txt").write_text("state")
    return legacy_dir


def _legacy_history_path(home: Path) -> Path:
    if sys.platform == "win32":
        return home / "AppData" / "Local" / "exasol_history"
    if sys.platform == "darwin":
        return home / "Library" / "Caches" / "exasol_history"
    return home / ".cache" / "exasol_history"


def _run(exasol_path: str, home: Path, *args: str) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        [str(Path(exasol_path).resolve()), *args],
        env=env_with_home(home),
        capture_output=True,
        text=True,
        check=False,
    )


@pytest.mark.openspec("launcher-path-migration", "deployment-directory-resolution")
def test_command_runs_against_a_migrated_legacy_deployment(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given a named deployment at the legacy location only
    home = tmp_path / "home"
    legacy_dir = _seed_legacy_deployment(home, "prod")

    # When a command selects that deployment by name
    result = _run(exasol_path, home, "status", "--json", "--deployment", "prod")

    # Then it is migrated first and the command resolves the new location
    assert result.returncode == 0, result.stderr
    new_dir = deployments_root(home) / "prod"
    assert f"Migrated managed deployments to {deployments_root(home)}" in result.stderr
    assert json.loads(result.stdout)["deploymentDir"] == str(new_dir)
    assert (new_dir / "marker.txt").read_text() == "state"
    assert not legacy_dir.exists()


@pytest.mark.openspec("launcher-path-migration")
def test_failed_migration_stops_before_the_command_runs(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given a deployment name present at both the legacy and the new location
    home = tmp_path / "home"
    legacy_dir = _seed_legacy_deployment(home, "prod")
    new_dir = deployments_root(home) / "prod"
    new_dir.mkdir(parents=True)

    # When any command runs
    result = _run(exasol_path, home, "status", "--json", "--deployment", "prod")

    # Then it fails naming both paths, and the command produces no output
    assert result.returncode != 0
    assert str(legacy_dir) in result.stderr
    assert str(new_dir) in result.stderr
    assert result.stdout == ""


@pytest.mark.openspec("sql-history-storage")
def test_legacy_history_file_is_migrated(exasol_path: str, tmp_path: Path) -> None:
    # Given a history file at the legacy location only
    home = tmp_path / "home"
    legacy_history = _legacy_history_path(home)
    legacy_history.parent.mkdir(parents=True)
    legacy_history.write_text("SELECT 1;\n")

    # When any command runs
    result = _run(exasol_path, home, "deployments", "list", "--json")

    # Then the history file is at the new location with its contents
    assert result.returncode == 0, result.stderr
    new_history = deployments_root(home).parent / "history" / "exasol_history"
    assert new_history.read_text() == "SELECT 1;\n"
    assert not legacy_history.exists()
