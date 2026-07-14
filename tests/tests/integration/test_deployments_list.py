# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

import json
import os
import subprocess
from pathlib import Path

from .helpers import first_infrastructure_preset_id_or_skip, run_command


def _env_with_home(home: Path) -> dict[str, str]:
    env = os.environ.copy()
    env["HOME"] = str(home)
    env["USERPROFILE"] = str(home)
    env["HOMEDRIVE"] = ""
    env["HOMEPATH"] = ""
    return env


def test_deployments_list_json_is_empty_array_when_none_exist(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given a fresh home directory with no deployments
    home = tmp_path / "home"
    home.mkdir()
    launcher = str(Path(exasol_path).resolve())

    # When deployments list is invoked
    result = subprocess.run(
        [launcher, "deployments", "list", "--json"],
        env=_env_with_home(home),
        capture_output=True,
        text=True,
        check=True,
    )

    # Then it succeeds with an empty JSON array, not null
    assert json.loads(result.stdout) == []


def test_deployments_list_reports_named_deployments(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given two named deployments, one initialized and one not
    home = tmp_path / "home"
    home.mkdir()
    infra_id = first_infrastructure_preset_id_or_skip(exasol_path)
    launcher = str(Path(exasol_path).resolve())
    run_command(
        [
            launcher,
            "init",
            infra_id,
            "--deployment",
            "staging",
            "--no-launcher-version-check",
        ],
        env=_env_with_home(home),
    )
    (home / ".exasol" / "personal" / "deployments" / "empty-named").mkdir(parents=True)

    # When deployments list is invoked with --json
    result = subprocess.run(
        [launcher, "deployments", "list", "--json"],
        env=_env_with_home(home),
        capture_output=True,
        text=True,
        check=True,
    )

    # Then it reports both entries, sorted alphabetically, with correct status
    entries = json.loads(result.stdout)
    names = [entry["name"] for entry in entries]
    assert names == sorted(names)
    assert "staging" in names
    assert "empty-named" in names

    staging = next(entry for entry in entries if entry["name"] == "staging")
    assert staging["status"] == "initialized"
    assert staging["infrastructure"]
    assert staging["installation"]

    empty_named = next(entry for entry in entries if entry["name"] == "empty-named")
    assert empty_named["status"] == "not_initialized"
    assert "infrastructure" not in empty_named


def test_deployments_list_marks_active_entry_from_current_directory(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given a named deployment
    home = tmp_path / "home"
    home.mkdir()
    infra_id = first_infrastructure_preset_id_or_skip(exasol_path)
    launcher = str(Path(exasol_path).resolve())
    named_dir = home / ".exasol" / "personal" / "deployments" / "staging"
    run_command(
        [
            launcher,
            "init",
            infra_id,
            "--deployment",
            "staging",
            "--no-launcher-version-check",
        ],
        env=_env_with_home(home),
    )

    # When deployments list is invoked from inside that deployment directory
    result = subprocess.run(
        [launcher, "deployments", "list", "--json"],
        cwd=named_dir,
        env=_env_with_home(home),
        capture_output=True,
        text=True,
        check=True,
    )

    # Then only that entry is marked active
    entries = json.loads(result.stdout)
    active_entries = [entry["name"] for entry in entries if entry["active"]]
    assert active_entries == ["staging"]


def test_deployments_list_does_not_accept_deployment_dir_or_deployment(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given a fresh home directory
    home = tmp_path / "home"
    home.mkdir()
    launcher = str(Path(exasol_path).resolve())

    # When deployments list is invoked with --deployment-dir or --deployment
    dir_result = subprocess.run(
        [launcher, "deployments", "list", "--deployment-dir", str(tmp_path)],
        env=_env_with_home(home),
        capture_output=True,
        text=True,
        check=False,
    )
    deployment_result = subprocess.run(
        [launcher, "deployments", "list", "--deployment", "staging"],
        env=_env_with_home(home),
        capture_output=True,
        text=True,
        check=False,
    )

    # Then both are rejected as unknown flags
    assert dir_result.returncode != 0
    assert "unknown flag" in dir_result.stderr
    assert deployment_result.returncode != 0
    assert "unknown flag" in deployment_result.stderr
