# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

import json
from pathlib import Path

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
