# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

import json
import subprocess
import sys
from pathlib import Path

import pytest

from .helpers import first_infrastructure_preset_id_or_skip, run_command


def test_help_flag(exasol_path: str) -> None:
    # When `exasol --help` is called
    result = run_command([exasol_path, "--help"])

    # Then the expected help and usage output is produced
    output = result.stdout.strip()
    for text in [
        "Exasol Personal",
        "Usage:",
        "Additional Commands:",
        "Essential Commands:",
        "Flags:",
        'Use "exasol [command] --help" for more information about a command.',
    ]:
        assert text in output


def test_help_flag_surfaces_local_preset_and_quick_start(exasol_path: str) -> None:
    # When `exasol --help` is called
    result = run_command([exasol_path, "--help"])

    # Then the local preset is listed first among the built-in presets
    output = result.stdout.strip()
    assert "Built-in presets are: local, aws, azure, exoscale, and stackit." in output

    # And a local quick-start pointer and deployment lifecycle are documented
    assert "exasol install local" in output
    assert (
        "Deployment lifecycle: install -> status -> connect -> stop -> start" in output
    )


def test_help_output_never_has_more_than_one_blank_line(exasol_path: str) -> None:
    # When --help is requested for a command with grouped subcommands and a leaf command
    root_help = run_command([exasol_path, "--help"]).stdout
    leaf_help = run_command([exasol_path, "status", "--help"]).stdout

    # Then no section is ever separated by more than a single blank line
    assert "\n\n\n" not in root_help
    assert "\n\n\n" not in leaf_help


def test_version(exasol_path: str) -> None:
    # Given the current version of the program based on the the latest git version tag
    git_describe_command_result = run_command(
        ["git", "describe", "--tags", "--abbrev=0"],
    )
    git_tag_version_str = git_describe_command_result.stdout.strip()

    # When I the run version command to print the program version
    version_command_result = run_command(
        [exasol_path, "version"],
    )
    version_command_output: str = version_command_result.stdout.strip()

    # Then I expect that the git tag version starts with a "v"
    version_expected_first_char = git_tag_version_str[0]
    assert version_expected_first_char == "v"

    # And I expect the version command output to be the same as the tagged version
    tag_expected_str = git_tag_version_str[1:]
    assert version_command_output == tag_expected_str


def test_version_json(exasol_path: str) -> None:
    # Given the current version of the program based on the the latest git version tag
    git_describe_command_result = run_command(
        ["git", "describe", "--tags", "--abbrev=0"],
    )
    git_tag_version_str = git_describe_command_result.stdout.strip()

    # When I run the version command with JSON output
    version_command_result = run_command(
        [exasol_path, "version", "--json"],
    )
    version_command_output = json.loads(version_command_result.stdout)

    # Then the version is returned as structured JSON
    assert version_command_output == {"version": git_tag_version_str[1:]}


def test_info_command_exists(exasol_path: str) -> None:
    """Verify info command is available."""
    result = run_command([exasol_path, "info", "--help"])
    assert result.returncode == 0
    assert "Prints information about your Exasol deployment." in result.stdout


def test_info_reports_missing_deployment_without_error(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given an empty deployment directory
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()

    # When info is invoked before a deployment exists
    result = run_command([exasol_path, "info", "--deployment-dir", str(deployment_dir)])

    # Then it exits successfully and guides the user toward creating a deployment
    assert "No Exasol Personal deployment exists" in result.stdout
    assert str(deployment_dir) in result.stdout
    assert "exasol install <infra preset>" in result.stdout
    assert "exasol presets list" in result.stdout


def test_info_json_reports_missing_deployment_without_error(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given an empty deployment directory
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()

    # When info is invoked as JSON before a deployment exists
    result = run_command(
        [exasol_path, "info", "--json", "--deployment-dir", str(deployment_dir)]
    )

    # Then automation can branch on structured state instead of parsing an error
    data = json.loads(result.stdout)
    assert data["deploymentState"] == "not_initialized"
    assert data["deploymentDir"] == str(deployment_dir)
    assert "message" not in data
    assert "actions" not in data


def test_info_command_init_deployment(exasol_path: str, tmp_path: Path) -> None:
    # Given an empty directory
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()

    # Given the deployment is initialized sucessfully
    infra_id = first_infrastructure_preset_id_or_skip(exasol_path)
    result = run_command(
        [
            exasol_path,
            "init",
            infra_id,
            "--deployment-dir",
            str(deployment_dir),
        ]
    )
    assert result.returncode == 0, (
        f"Init failed with return code {result.returncode}\nError: {result.stderr}"
    )

    # When the info command is being invoked
    result = run_command([exasol_path, "info", "--deployment-dir", str(deployment_dir)])

    # Then it indicates that it's ready for deployment
    assert "Ready for deployment" in result.stdout


@pytest.mark.launcher_tests
@pytest.mark.parametrize("command", ["destroy", "remove", "install"])
def test_help_uses_inclusive_phrasing(exasol_path: str, command: str) -> None:
    # When the command help is rendered
    output = run_command([exasol_path, command, "--help"]).stdout.lower()

    # Then it never uses provider-specific "cloud resources" phrasing
    assert "cloud resources" not in output


@pytest.mark.launcher_tests
def test_destroy_help_mentions_deployment_resources(exasol_path: str) -> None:
    # When destroy help is rendered
    output = run_command([exasol_path, "destroy", "--help"]).stdout.lower()

    # Then it describes generic "deployment resources"
    assert "deployment resources" in output


@pytest.mark.skipif(
    not sys.platform.startswith("win"), reason="Windows-only shell-equivalence test"
)
def test_help_consistent_in_powershell_and_cmd(exasol_path: str) -> None:
    """--help output and exit code must match between PowerShell and CMD."""
    # Given the launcher on Windows
    # When --help is run via PowerShell and via cmd.exe
    powershell = subprocess.run(
        ["powershell.exe", "-NoProfile", "-Command", exasol_path, "--help"],  # noqa: S607
        capture_output=True,
        text=True,
        check=False,
    )
    cmd = subprocess.run(
        ["cmd.exe", "/c", exasol_path, "--help"],  # noqa: S607
        capture_output=True,
        text=True,
        check=False,
    )

    # Then both shells return success and the same help text
    assert powershell.returncode == 0
    assert cmd.returncode == 0
    assert powershell.stdout.strip() == cmd.stdout.strip()


def test_unknown_flag_exits_nonzero_with_usage(exasol_path: str) -> None:
    """Unsupported flag must exit non-zero with usage and no stack trace."""
    # Given the launcher binary
    # When called with a flag that does not exist
    proc = subprocess.run(
        [exasol_path, "--definitely-not-a-flag"],
        capture_output=True,
        text=True,
        check=False,
    )

    # Then it exits non-zero and surfaces usage without a stack trace
    assert proc.returncode != 0
    combined = (proc.stdout + proc.stderr).lower()
    assert "unknown flag" in combined or "usage" in combined
    assert "traceback" not in combined
