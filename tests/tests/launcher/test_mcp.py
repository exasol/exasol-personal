# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

import json
from pathlib import Path
from subprocess import CalledProcessError

import pytest

from .helpers import run_command

pytestmark = pytest.mark.openspec("mcp-command")


def init_local(exasol_path: str, directory: Path) -> list[str]:
    run_command(
        [
            exasol_path,
            "init",
            "local",
            "--deployment-dir",
            str(directory),
            "--no-launcher-version-check",
        ]
    )
    return ["--deployment-dir", str(directory), "--json"]


def test_help_lists_the_mcp_command(exasol_path: str) -> None:
    # When
    output = run_command([exasol_path, "--help"]).stdout
    # Then
    assert "mcp         Connect AI agents" in output


def test_mcp_subcommands_match_the_sidecar_commands(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given
    via_mcp = init_local(exasol_path, tmp_path / "mcp")
    via_sidecar = init_local(exasol_path, tmp_path / "sidecar")
    steps = [
        (["mcp", "enable"], ["sidecar", "enable", "mcp"]),
        (["mcp", "status"], ["sidecar", "status", "mcp"]),
        (["mcp", "disable", "--remove"], ["sidecar", "disable", "mcp", "--remove"]),
    ]
    for mcp_command, sidecar_command in steps:
        # When
        mcp_result = run_command([exasol_path, *mcp_command, *via_mcp]).stdout
        sidecar_result = run_command([exasol_path, *sidecar_command, *via_sidecar])
        # Then
        assert json.loads(mcp_result) == json.loads(sidecar_result.stdout)


def test_mcp_instructions_lists_clients(exasol_path: str, tmp_path: Path) -> None:
    # Given
    selection = init_local(exasol_path, tmp_path / "deployment")
    # When
    text = run_command([exasol_path, "mcp", "instructions", *selection[:2]]).stdout
    clients = json.loads(
        run_command([exasol_path, "mcp", "instructions", *selection]).stdout
    )
    # Then
    assert "claude-code      Claude Code" in text
    assert [client["id"] for client in clients] == [
        "claude-code",
        "claude-desktop",
        "codex",
        "cursor",
        "vscode",
        "gemini",
        "opencode",
        "continue",
    ]


def test_mcp_instructions_prints_client_configuration(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given
    directory = tmp_path / "deployment"
    selection = init_local(exasol_path, directory)
    run_command([exasol_path, "mcp", "enable", *selection])
    sidecars = directory / "sidecars.yaml"
    sidecars.write_text(
        sidecars.read_text(encoding="utf-8").replace(
            "hostPort: 4896", "hostPort: 14896"
        ),
        encoding="utf-8",
    )
    endpoint = "http://127.0.0.1:14896/mcp"
    # When
    command = run_command(
        [exasol_path, "mcp", "instructions", "claude-code", *selection[:2]]
    )
    file = json.loads(
        run_command([exasol_path, "mcp", "instructions", "cursor", *selection]).stdout
    )
    # Then
    assert (
        f"claude mcp add --transport http --scope user exasol-personal {endpoint}"
        in command.stdout
    )
    assert file["url"] == endpoint
    assert file["file"] == "~/.cursor/mcp.json"
    assert json.loads(file["content"]) == {
        "mcpServers": {"exasol-personal": {"url": endpoint}}
    }


def test_mcp_instructions_requires_an_enabled_server(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given
    selection = init_local(exasol_path, tmp_path / "deployment")
    # When
    with pytest.raises(CalledProcessError) as failure:
        run_command([exasol_path, "mcp", "instructions", "codex", *selection[:2]])
    # Then
    assert "the MCP server is not enabled" in failure.value.stderr
    assert "exasol mcp enable" in failure.value.stderr


def test_mcp_instructions_explain_a_missing_deployment(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given
    directory = tmp_path / "missing"
    # When
    with pytest.raises(CalledProcessError) as failure:
        run_command(
            [
                exasol_path,
                "mcp",
                "instructions",
                "claude-code",
                "--deployment-dir",
                str(directory),
            ]
        )
    # Then
    stderr = failure.value.stderr
    assert "no initialized deployment was found in" in stderr
    assert directory.name in stderr
    assert "use the selected deployment's MCP endpoint" in stderr
    assert "exasol install local" in stderr
    assert "exasol mcp instructions claude-code -d NAME" in stderr
