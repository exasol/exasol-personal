# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

from pathlib import Path

import pytest

from scripts import cli_reference

INSTALL_HELP = """Initialize, apply configuration, and deploy Exasol in one step

	Tip: use "exasol presets" to discover and export presets.

Usage:
	exasol install <infra preset name-or-path> [flags]

Global Flags:
      --log-level string   Set log level

Infrastructure variable flags of preset `aws`:
      --cluster-size number   Number of nodes in the cluster (default: 1)

Installation variable flags of preset `ubuntu`:
      --exasol-version string   Exasol database version to install

Flags:
  -v, --verbose   Enable verbose output

Examples:
  exasol install aws
"""


@pytest.mark.parametrize(
    "line",
    ["Usage:", "Global Flags:", "Infrastructure variable flags of preset `aws`:"],
)
def test_section_headers_start_at_the_margin(line: str) -> None:
    # Given / When / Then
    assert cli_reference.is_section_header(line)


@pytest.mark.parametrize(
    "line",
    ["", "      --log-level string   Set log level", "	Tip: use presets:", "plain"],
)
def test_indented_and_unterminated_lines_are_not_section_headers(line: str) -> None:
    # Given / When / Then
    assert not cli_reference.is_section_header(line)


def test_preset_flags_keeps_only_the_preset_variable_sections() -> None:
    # Given / When
    actual = cli_reference.preset_flags(INSTALL_HELP)

    # Then
    assert actual == (
        "Infrastructure variable flags of preset `aws`:\n"
        "      --cluster-size number   Number of nodes in the cluster (default: 1)\n"
        "\n"
        "Installation variable flags of preset `ubuntu`:\n"
        "      --exasol-version string   Exasol database version to install"
    )


def test_preset_flags_is_empty_without_preset_variables() -> None:
    # Given
    help_text = "Usage:\n\texasol status [flags]\n\nFlags:\n  -j, --json   JSON\n"

    # When
    actual = cli_reference.preset_flags(help_text)

    # Then
    assert actual == ""


def test_global_flags_drops_the_per_command_help_line() -> None:
    # Given / When
    actual = cli_reference.global_flags(
        "Global Flags:\n"
        "      --log-level string   Set log level\n"
        "  -h, --help               Help for install\n"
    )

    # Then
    assert actual == "      --log-level string   Set log level"


def test_global_flags_is_empty_without_a_global_section() -> None:
    # Given / When
    actual = cli_reference.global_flags("Usage:\n\texasol status\n")

    # Then
    assert actual == ""


def test_strip_repeated_removes_global_flags_and_the_navigation_trailer() -> None:
    # Given
    help_text = (
        "Manage deployment configuration\n"
        "\n"
        "Usage:\n"
        "\texasol config [command] [flags]\n"
        "\n"
        "Global Flags:\n"
        "      --log-level string   Set log level\n"
        "\n"
        'Use "exasol config [command] --help" for more information about a command.\n'
    )

    # When
    actual = cli_reference.strip_repeated(help_text)

    # Then
    assert actual == (
        "Manage deployment configuration\n\nUsage:\n\texasol config [command] [flags]"
    )


def test_strip_repeated_keeps_command_specific_flags() -> None:
    # Given
    help_text = "Flags:\n  -j, --json   Output in JSON format\n"

    # When
    actual = cli_reference.strip_repeated(help_text)

    # Then
    assert actual == "Flags:\n  -j, --json   Output in JSON format"


@pytest.mark.parametrize(
    ("path", "expected"),
    [
        (("completion", "bash"), True),
        (("completion",), False),
        (("slc", "install"), False),
        ((), False),
    ],
)
def test_only_nested_boilerplate_commands_are_collapsed(
    path: tuple[str, ...], *, expected: bool
) -> None:
    # Given / When / Then
    assert cli_reference.is_collapsed(path) is expected


def test_command_groups_follow_the_launchers_own_grouping() -> None:
    # Given
    root_help = (
        "Exasol Personal\n"
        "\n"
        "Usage:\n"
        "\texasol [command] [flags]\n"
        "\n"
        "Essential Commands:\n"
        "\tinstall     Initialize and deploy\n"
        "\tinit        Initialize a deployment directory\n"
        "\n"
        "Lifecycle Commands:\n"
        "\tstart       Start a deployment\n"
        "\n"
        "Global Flags:\n"
        "      --log-level string   Set log level\n"
    )

    # When
    actual = cli_reference.command_groups(root_help)

    # Then
    assert actual == [
        ("Essential Commands", ["install", "init"]),
        ("Lifecycle Commands", ["start"]),
    ]


@pytest.mark.parametrize(
    ("path", "expected"),
    [
        ((), "exasol"),
        (("status",), "status"),
        (("slc", "custom", "install"), "slc custom install"),
    ],
)
def test_toc_label_drops_the_repeated_program_name(
    path: tuple[str, ...], expected: str
) -> None:
    # Given / When / Then
    assert cli_reference.toc_label(path) == expected


def test_block_carries_the_table_of_contents_label() -> None:
    # Given / When
    actual = cli_reference.block("###", "exasol slc list", "usage", "slc list")

    # Then
    assert actual.startswith('### `exasol slc list` { data-toc-label="slc list" }')


def test_collapsed_block_indents_the_whole_fence() -> None:
    # Given / When
    actual = cli_reference.collapsed_block("exasol completion bash", "usage\n\nmore")

    # Then
    assert actual == (
        '??? note "`exasol completion bash`"\n'
        "\n"
        "    ```text\n"
        "    usage\n"
        "\n"
        "    more\n"
        "    ```\n"
    )


def test_normalize_replaces_the_home_directory(tmp_path: Path) -> None:
    # Given
    text = f"uses {tmp_path}/.exasol/personal/deployments/default\n"

    # When
    actual = cli_reference.normalize(text, tmp_path)

    # Then
    assert actual == "uses <home>/.exasol/personal/deployments/default"


def test_check_accepts_a_reference_that_matches_the_binary(
    monkeypatch: pytest.MonkeyPatch, tmp_path: Path
) -> None:
    # Given
    reference = tmp_path / "cli-reference.md"
    reference.write_text("generated\n", encoding="utf-8")
    monkeypatch.setattr(cli_reference, "generate", lambda _: "generated\n")

    # When / Then
    cli_reference.check(Path("exasol"), reference)


def test_check_rejects_a_reference_that_drifted_from_the_binary(
    monkeypatch: pytest.MonkeyPatch, tmp_path: Path
) -> None:
    # Given
    reference = tmp_path / "cli-reference.md"
    reference.write_text("stale\n", encoding="utf-8")
    monkeypatch.setattr(cli_reference, "generate", lambda _: "generated\n")

    # When / Then
    with pytest.raises(cli_reference.CliReferenceError, match="does not match"):
        cli_reference.check(Path("exasol"), reference)


def test_check_reports_a_missing_reference(tmp_path: Path) -> None:
    # Given
    missing = tmp_path / "cli-reference.md"

    # When / Then
    with pytest.raises(cli_reference.CliReferenceError, match="not found"):
        cli_reference.check(Path("exasol"), missing)


def test_generate_reports_a_missing_binary(tmp_path: Path) -> None:
    # Given
    missing = tmp_path / "exasol"

    # When / Then
    with pytest.raises(cli_reference.CliReferenceError, match="not found"):
        cli_reference.generate(missing)
