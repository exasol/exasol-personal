# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

from pathlib import Path

from .helpers import first_infrastructure_preset_id_or_skip, run_command


def test_help_flag(exasol_path: str) -> None:
    # When `exasol --help` is called
    result = run_command([exasol_path, "--help"])

    # Then the expected help and usage output is produced
    output = result.stdout.strip()
    for text in [
        "Exasol Personal Launcher",
        "Usage:",
        "Additional Commands:",
        "Essential Commands:",
        "Flags:",
        'Use "exasol [command] --help" for more information about a command.',
    ]:
        assert text in output


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


def test_info_command_exists(exasol_path: str) -> None:
    """Verify info command is available."""
    result = run_command([exasol_path, "info", "--help"])
    assert result.returncode == 0
    assert "Prints a summary" in result.stdout


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
