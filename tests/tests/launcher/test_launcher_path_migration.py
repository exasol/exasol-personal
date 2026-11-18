# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

import json
import subprocess
import sys
from datetime import UTC, datetime, timedelta
from pathlib import Path

import pytest

from .helpers import deployments_root, env_with_home


def _seed_legacy_deployment(home: Path, name: str) -> Path:
    legacy_dir = home / ".exasol" / "personal" / "deployments" / name
    legacy_dir.mkdir(parents=True)
    (legacy_dir / "marker.txt").write_text("state")
    return legacy_dir


def _seed_kept_legacy_deployment(home: Path, name: str) -> Path:
    legacy_dir = _seed_legacy_deployment(home, name)
    (legacy_dir / ".dirmutex.shared.1").touch()
    return legacy_dir


def _legacy_history_path(home: Path) -> Path:
    if sys.platform == "win32":
        return home / "AppData" / "Local" / "exasol_history"
    if sys.platform == "darwin":
        return home / "Library" / "Caches" / "exasol_history"
    return home / ".cache" / "exasol_history"


def _run(
    exasol_path: str, home: Path, *args: str, cwd: Path | None = None
) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        [str(Path(exasol_path).resolve()), *args],
        cwd=cwd,
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


@pytest.mark.openspec("deployment-directory-resolution")
def test_explicit_legacy_path_follows_the_migrated_deployment(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given a named deployment at the legacy location only
    home = tmp_path / "home"
    legacy_dir = _seed_legacy_deployment(home, "prod")

    # When a command selects it by its legacy path
    result = _run(
        exasol_path, home, "status", "--json", "--deployment-dir", str(legacy_dir)
    )

    # Then it is migrated first and the command uses the new location
    assert result.returncode == 0, result.stderr
    new_dir = deployments_root(home) / "prod"
    assert f"Using migrated deployment directory: {new_dir}" in result.stderr
    assert json.loads(result.stdout)["deploymentDir"] == str(new_dir)
    assert not legacy_dir.exists()


@pytest.mark.openspec("deployment-directory-resolution")
def test_relative_legacy_path_follows_the_migrated_deployment(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given a named deployment at the legacy location only
    home = tmp_path / "home"
    legacy_dir = _seed_legacy_deployment(home, "prod")

    # When a command selects it relative to the legacy deployments folder
    result = _run(
        exasol_path,
        home,
        "status",
        "--json",
        "--deployment-dir",
        "prod",
        cwd=legacy_dir.parent,
    )

    # Then it is migrated first and the command uses the new location
    assert result.returncode == 0, result.stderr
    new_dir = deployments_root(home) / "prod"
    assert json.loads(result.stdout)["deploymentDir"] == str(new_dir)
    assert legacy_dir.parent.is_dir()


@pytest.mark.openspec("launcher-path-migration", "deployment-directory-resolution")
def test_kept_legacy_deployment_warns_and_the_command_runs(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given a deployment name present at both the legacy and the new location
    home = tmp_path / "home"
    legacy_dir = _seed_legacy_deployment(home, "prod")
    new_dir = deployments_root(home) / "prod"
    new_dir.mkdir(parents=True)

    # When a command selects that deployment by name
    result = _run(exasol_path, home, "status", "--json", "--deployment", "prod")

    # Then it warns naming both paths and runs against the new location
    assert result.returncode == 0, result.stderr
    assert f"Warning: kept {legacy_dir}" in result.stderr
    assert str(new_dir) in result.stderr
    assert json.loads(result.stdout)["deploymentDir"] == str(new_dir)
    assert (legacy_dir / "marker.txt").read_text() == "state"


@pytest.mark.openspec("deployment-directory-resolution")
def test_named_selection_uses_kept_legacy_deployment(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given a named deployment kept at the legacy location
    home = tmp_path / "home"
    legacy_dir = _seed_kept_legacy_deployment(home, "prod")

    # When status selects the deployment by name
    result = _run(exasol_path, home, "status", "--json", "--deployment", "prod")

    # Then status uses the legacy path with its ordinary selected-directory notice
    assert result.returncode == 0, result.stderr
    assert json.loads(result.stdout)["deploymentDir"] == str(legacy_dir)
    notice = f'Using named deployment directory "prod": {legacy_dir}'
    assert result.stderr.count(notice) == 1
    assert legacy_dir.is_dir()


@pytest.mark.openspec("deployment-directory-resolution")
def test_implicit_default_selection_uses_kept_legacy_deployment(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given a default deployment kept at the legacy location and an unrelated cwd
    home = tmp_path / "home"
    legacy_dir = _seed_kept_legacy_deployment(home, "default")
    cwd = tmp_path / "work"
    cwd.mkdir()

    # When status selects the implicit default deployment
    result = _run(exasol_path, home, "status", "--json", cwd=cwd)

    # Then status uses the legacy path with its ordinary selected-directory notice
    assert result.returncode == 0, result.stderr
    assert json.loads(result.stdout)["deploymentDir"] == str(legacy_dir)
    prefix = "Using default deployment directory: "
    notice = f"{prefix}{legacy_dir}"
    assert result.stderr.count(notice) == 1
    assert legacy_dir.is_dir()


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


@pytest.mark.openspec("launcher-path-migration", "deployment-directory-resolution")
def test_daily_migration_guidance_survives_json_and_is_not_repeated(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given a locked legacy deployment whose ordinary selection remains usable
    home = tmp_path / "home"
    legacy_dir = _seed_kept_legacy_deployment(home, "prod")

    # When JSON runs before the day's text invocations
    structured = _run(exasol_path, home, "status", "--json", "-d", "prod")
    assert structured.returncode == 0, structured.stderr
    assert json.loads(structured.stdout)["deploymentDir"] == str(legacy_dir)
    assert "Migration pending" not in structured.stderr
    assert not list(legacy_dir.glob(".launcher-path-migration-reminder-*"))
    first = _run(exasol_path, home, "deployments", "list")
    repeated = _run(exasol_path, home, "status", "-d", "prod")

    # Then redirected text output gets one CTA on stderr for that deployment
    assert first.returncode == 0, first.stderr
    assert repeated.returncode == 0, repeated.stderr
    assert first.stderr.count(f"Migration pending for {legacy_dir}") == 1
    assert "Support for the old location is temporary" in first.stderr
    assert "If none is running" in first.stderr
    assert str(deployments_root(home) / "prod") in first.stderr
    assert "Migration pending" not in first.stdout
    assert "Migration pending" not in repeated.stderr
    assert "Warning: Using" not in repeated.stderr


@pytest.mark.openspec("launcher-path-migration")
def test_daily_migration_guidance_uses_each_deployments_saved_date(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given a previous-day claim and a future claim on separate legacy deployments
    home = tmp_path / "home"
    due = _seed_kept_legacy_deployment(home, "due")
    future = _seed_kept_legacy_deployment(home, "future")
    today = datetime.now(tz=UTC).astimezone().date()
    yesterday = today - timedelta(days=1)
    tomorrow = today + timedelta(days=1)
    old_marker = due / f".launcher-path-migration-reminder-{yesterday.isoformat()}"
    old_marker.touch()
    (future / f".launcher-path-migration-reminder-{tomorrow.isoformat()}").touch()

    # When an invocation encounters both deployments
    result = _run(exasol_path, home, "deployments", "list")

    # Then only the due deployment receives guidance and older state is retired
    assert result.returncode == 0, result.stderr
    assert f"Migration pending for {due}" in result.stderr
    assert f"Migration pending for {future}" not in result.stderr
    assert not old_marker.exists()


@pytest.mark.openspec("launcher-path-migration")
def test_a_daily_reminder_does_not_delay_later_migration(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given a legacy deployment that already received today's reminder
    home = tmp_path / "home"
    legacy_dir = _seed_kept_legacy_deployment(home, "prod")
    first = _run(exasol_path, home, "deployments", "list")
    assert first.returncode == 0, first.stderr
    assert "Migration pending" in first.stderr
    (legacy_dir / ".dirmutex.shared.1").unlink()

    # When the next command retries after the lock is released
    result = _run(exasol_path, home, "status", "--json", "-d", "prod")

    # Then it moves immediately and retires migration-specific reminder state
    current = deployments_root(home) / "prod"
    assert result.returncode == 0, result.stderr
    assert json.loads(result.stdout)["deploymentDir"] == str(current)
    assert not legacy_dir.exists()
    assert (current / "marker.txt").read_text() == "state"
    assert not list(current.glob(".launcher-path-migration-reminder-*"))


@pytest.mark.openspec("launcher-path-migration")
@pytest.mark.parametrize(
    "args",
    [("status", "--unknown-option"), ("status", "--help"), ("completion", "bash")],
)
def test_rejected_commands_help_and_completion_preserve_the_daily_reminder(
    exasol_path: str, tmp_path: Path, args: tuple[str, ...]
) -> None:
    # Given a legacy deployment whose reminder has not yet been shown
    home = tmp_path / "home"
    legacy_dir = _seed_kept_legacy_deployment(home, "prod")

    # When a command is rejected or only renders help/completion
    _run(exasol_path, home, *args)

    # Then the next ordinary text invocation still gets its daily reminder
    assert not list(legacy_dir.glob(".launcher-path-migration-reminder-*"))
    result = _run(exasol_path, home, "deployments", "list")
    assert result.returncode == 0, result.stderr
    assert f"Migration pending for {legacy_dir}" in result.stderr
