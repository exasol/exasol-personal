# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

import json
import os
import stat
import subprocess
import time
from pathlib import Path
from subprocess import CalledProcessError

import pytest

from .conftest import get_version_check_count
from .helpers import (
    export_preset,
    first_infrastructure_preset_id_or_skip,
    installation_preset_id_or_skip,
    run_command,
)


def test_init_defaults_and_help(exasol_path: str) -> None:
    # Given the init command

    # When I call `exasol init --help`
    result = run_command([exasol_path, "init", "--help"])
    output: str = result.stdout.strip()

    # Then I see documentation for the init command
    assert "Initialize a new deployment directory" in output

    # Then I see which preset names I can pass
    infra_id = first_infrastructure_preset_id_or_skip(exasol_path)
    assert "Available infrastructure presets:" in output
    assert infra_id in output
    assert "Available installation presets:" in output

    # And the help nudges users towards presets discovery/export
    assert "exasol presets" in output


def test_init_requires_infra_preset_arg(exasol_path: str) -> None:
    # When the init command is invoked without arguments
    with pytest.raises(CalledProcessError) as exc:
        run_command([exasol_path, "init"])

    # Then it fails because the required infra preset argument is missing
    assert exc.value.returncode != 0
    assert (
        "requires" in (exc.value.stderr or "").lower()
        or "accepts" in (exc.value.stderr or "").lower()
    )


def test_init_succeeds(exasol_path: str, tmp_path: Path) -> None:
    # Given an empty deployment directory
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()

    # Given an infrastructure preset ID
    infra_id = first_infrastructure_preset_id_or_skip(exasol_path)

    # When `exasol init` is invoked
    result = run_command(
        [
            exasol_path,
            "init",
            infra_id,
            "--deployment-dir",
            str(deployment_dir),
        ]
    )

    # Then the command succeeds
    assert result.returncode == 0
    assert "successfully initialized deployment" in result.stderr.lower()

    # Then the launcher state file exists
    assert (deployment_dir / ".exasolLauncherState.json").exists()

    # Then and the EULA is there too
    assert (deployment_dir / "eula.txt").exists()

    # Then the EULA notice is displayed as the final terminal notice, not as log output
    assert "End User License Agreement" in result.stderr or "EULA" in result.stderr
    assert "exasol.com" in result.stderr.lower()
    assert "End User License Agreement" not in result.stdout
    assert result.stderr.rfind("deployment log file") < result.stderr.rfind(
        "End User License Agreement"
    )
    assert result.stderr.strip().splitlines()[-1] == (
        "A copy of the EULA is also included as 'eula.txt' in this directory."
    )
    deployment_log = (deployment_dir / "deployment.log").read_text()
    assert "End User License Agreement" not in deployment_log


def test_init_creates_deployment_dir(exasol_path: str, tmp_path: Path) -> None:
    # Given an deployment directory path
    deployment_dir = tmp_path / "deployment"

    # Given an infrastructure preset ID
    infra_id = first_infrastructure_preset_id_or_skip(exasol_path)

    # When `exasol init` is invoked
    result = run_command(
        [
            exasol_path,
            "init",
            infra_id,
            "--deployment-dir",
            str(deployment_dir),
        ]
    )

    # Then the command succeeds
    assert result.returncode == 0
    assert "successfully initialized deployment" in result.stderr.lower()
    assert deployment_dir.exists()


def test_init_allows_deployment_dir_flag_before_preset_arg(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given an empty deployment directory
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()

    # Given an infrastructure preset ID
    infra_id = first_infrastructure_preset_id_or_skip(exasol_path)

    # When `--deployment-dir` is provided before the preset argument
    result = run_command(
        [
            exasol_path,
            "init",
            "--deployment-dir",
            str(deployment_dir),
            infra_id,
        ]
    )

    # Then init still succeeds
    assert result.returncode == 0
    assert (deployment_dir / ".exasolLauncherState.json").exists()


def test_init_fails_on_non_empty_directory(exasol_path: str, tmp_path: Path) -> None:
    # Given a non-empty deployment directory
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()

    # Given a random other file in the directory
    (deployment_dir / "somefile.txt").write_text("test content")

    # Given an infrastructure preset ID
    infra_id = first_infrastructure_preset_id_or_skip(exasol_path)

    # When `exasol init` is invoked
    args = [
        exasol_path,
        "init",
        infra_id,
        "--deployment-dir",
        str(deployment_dir),
    ]
    with pytest.raises(CalledProcessError) as exc:
        run_command(args)

    # Then the command fails
    assert exc.value.returncode != 0

    # Then the command output indicates that it fails because the directory wasn't empty
    assert "deployment directory is not empty" in exc.value.stderr.lower()


def test_init_idempotent(exasol_path: str, tmp_path: Path) -> None:
    # Given an empty deployment directory
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()

    # Given an infrastructure preset ID
    infra_id = first_infrastructure_preset_id_or_skip(exasol_path)

    # When `exasol init` is invoked
    result1 = run_command(
        [
            exasol_path,
            "init",
            infra_id,
            "--deployment-dir",
            str(deployment_dir),
        ]
    )
    # Then the command succeeds
    assert result1.returncode == 0
    # Then the command reports that the deployment is initialized
    assert "successfully initialized deployment" in result1.stderr.lower()

    # Given we save the state file stats
    launcher_state_file = deployment_dir / ".exasolLauncherState.json"
    original_workflow_mtime = launcher_state_file.stat().st_mtime

    # When `exasol init` is invoked again
    result2 = run_command(
        [
            exasol_path,
            "init",
            infra_id,
            "--deployment-dir",
            str(deployment_dir),
        ]
    )
    # Then the command succeeds too
    assert result2.returncode == 0
    # Then the command reports that the deployment was already initialized
    assert "already initialized" in result2.stderr.lower()
    # Then the state file wasn't modified
    assert launcher_state_file.stat().st_mtime == original_workflow_mtime


def test_init_accepts_infra_preset_path(exasol_path: str, tmp_path: Path) -> None:
    # Given an infrastructure preset exported to a directory
    infra_id = first_infrastructure_preset_id_or_skip(exasol_path)

    infra_dir = tmp_path / "infra_export"
    infra_dir.mkdir()
    export_preset(exasol_path, infra_id, "infrastructure", str(infra_dir))

    # Given an empty deployment directory
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()

    # When init is invoked with the infra preset directory path
    result = run_command(
        [
            exasol_path,
            "init",
            str(infra_dir),
            "--deployment-dir",
            str(deployment_dir),
        ]
    )

    # Then it succeeds
    assert result.returncode == 0
    assert (deployment_dir / ".exasolLauncherState.json").exists()


def test_init_accepts_install_preset_path_as_second_arg(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given an installation preset exported to a directory
    infra_id = first_infrastructure_preset_id_or_skip(exasol_path)
    install_id = installation_preset_id_or_skip(exasol_path, "ubuntu")

    install_dir = tmp_path / "install_export"
    install_dir.mkdir()
    export_preset(exasol_path, install_id, "installation", str(install_dir))

    # Given an empty deployment directory
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()

    # When init is invoked with the install preset directory path as second arg
    result = run_command(
        [
            exasol_path,
            "init",
            infra_id,
            str(install_dir),
            "--deployment-dir",
            str(deployment_dir),
        ]
    )

    assert result.returncode == 0
    assert "successfully initialized" in result.stderr.lower()

    # Get modification time of workflow state file
    launcher_state_file = deployment_dir / ".exasolLauncherState.json"
    original_workflow_mtime = launcher_state_file.stat().st_mtime

    # Small delay to ensure different timestamps if files were modified
    time.sleep(0.01)

    # Second init
    run_command(
        [
            exasol_path,
            "init",
            infra_id,
            str(install_dir),
            "--deployment-dir",
            str(deployment_dir),
        ]
    )

    # Verify workflow state file wasn't modified
    assert launcher_state_file.stat().st_mtime == original_workflow_mtime


def test_init_performs_version_check(
    exasol_path: str, tmp_path: Path, mock_version_server: str
) -> None:
    """Verify init command attempts a version check."""
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()

    endpoint = f"{mock_version_server}/version-check"

    # Run init with mock server endpoint
    result = run_command(
        [
            exasol_path,
            "init",
            "aws",
            "--deployment-dir",
            str(deployment_dir),
        ],
        env={**os.environ, "EXASOL_VERSION_CHECK_URL": endpoint},
    )

    assert result.returncode == 0
    assert "successfully initialized" in result.stderr.lower()

    # Check that version check was attempted
    count = get_version_check_count(mock_version_server)
    assert count == 1, f"Expected 1 version check, but got {count}"

    result = run_command(
        [
            exasol_path,
            "status",
            "--deployment-dir",
            str(deployment_dir),
        ],
        env={**os.environ, "EXASOL_VERSION_CHECK_URL": endpoint},
    )

    assert result.returncode == 0

    # Check that version check was not attempted again
    count = get_version_check_count(mock_version_server)
    assert count == 0, f"Expected no version check, but got {count}"

    # Remove lastVersionCheck from the state file to simulate missing version check info
    state_file = deployment_dir / ".exasolLauncherState.json"
    state_data = json.loads(state_file.read_text())
    state_data.pop("lastVersionCheck", None)
    state_file.write_text(json.dumps(state_data))

    result = run_command(
        [
            exasol_path,
            "status",
            "--deployment-dir",
            str(deployment_dir),
        ],
        env={**os.environ, "EXASOL_VERSION_CHECK_URL": endpoint},
    )

    assert result.returncode == 0

    # Check that version check was attempted again (since lastVersionCheck was removed)
    count = get_version_check_count(mock_version_server)
    assert count == 1, f"Expected 1 version check, but got {count}"


def test_init_skips_version_check(
    exasol_path: str, tmp_path: Path, mock_version_server: str
) -> None:
    """Verify init command skips the version check if configured."""
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()

    endpoint = f"{mock_version_server}/version-check"

    # Run init with mock server endpoint
    result = run_command(
        [
            exasol_path,
            "init",
            "aws",
            "--no-launcher-version-check",
            "--deployment-dir",
            str(deployment_dir),
        ],
        env={**os.environ, "EXASOL_VERSION_CHECK_URL": endpoint},
    )

    assert result.returncode == 0
    assert "successfully initialized" in result.stderr.lower()

    # Check that version check was not attempted
    count = get_version_check_count(mock_version_server)
    assert count == 0, f"Expected no version check, but got {count}"

    result = run_command(
        [
            exasol_path,
            "status",
            "--deployment-dir",
            str(deployment_dir),
        ],
        env={**os.environ, "EXASOL_VERSION_CHECK_URL": endpoint},
    )

    assert result.returncode == 0

    # Check that version check was not attempted again
    count = get_version_check_count(mock_version_server)
    assert count == 0, f"Expected no version check, but got {count}"

    # Enable version check in the state file to simulate user
    # enabling it after initialization
    state_file = deployment_dir / ".exasolLauncherState.json"
    state_data = json.loads(state_file.read_text())
    state_data["versionCheckEnabled"] = True
    state_file.write_text(json.dumps(state_data))

    result = run_command(
        [
            exasol_path,
            "status",
            "--deployment-dir",
            str(deployment_dir),
        ],
        env={**os.environ, "EXASOL_VERSION_CHECK_URL": endpoint},
    )

    assert result.returncode == 0

    # Check that version check was attempted (since lastVersionCheck was removed)
    count = get_version_check_count(mock_version_server)
    assert count == 1, f"Expected 1 version check, but got {count}"


def test_init_in_non_writable_dir_fails_with_clear_error(
    exasol_path: str,
    tmp_path: Path,
) -> None:
    """Init into a non-writable directory must fail with an actionable error.

    Note: the launcher writes most state under --deployment-dir, plus a readline
    history file under $XDG_CACHE_HOME. This test exercises the former: the
    deployment directory itself is read-only.
    """
    # Given a deployment directory the user cannot write to
    deployment_dir = tmp_path / "locked"
    deployment_dir.mkdir()
    deployment_dir.chmod(stat.S_IRUSR | stat.S_IXUSR)

    infra_id = first_infrastructure_preset_id_or_skip(exasol_path)

    # When init is invoked against it
    try:
        proc = subprocess.run(
            [
                exasol_path,
                "init",
                infra_id,
                "--deployment-dir",
                str(deployment_dir),
            ],
            capture_output=True,
            text=True,
            check=False,
        )

        # Then init exits non-zero with a message that references the path or
        # a permission problem, not a panic / Python traceback
        assert proc.returncode != 0
        combined = (proc.stdout + proc.stderr).lower()
        assert "panic:" not in combined
        assert "traceback" not in combined
        assert (
            "permission" in combined
            or "denied" in combined
            or "read-only" in combined
            or str(deployment_dir).lower() in combined
        ), f"Error did not name the path or permission cause: {combined!r}"
    finally:
        # Restore permissions so tmp_path cleanup can remove the directory
        deployment_dir.chmod(stat.S_IRWXU)
