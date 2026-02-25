# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

"""Helper utilities for integration tests."""

import json
import os
import subprocess
from subprocess import CompletedProcess

import pytest


def run_command(
    command: list[str], env: dict[str, str] | None = None
) -> CompletedProcess[str]:
    """Run CLI commands in integration tests."""
    return subprocess.run(
        command,
        capture_output=True,
        text=True,
        check=True,
        env=env if env is not None else os.environ.copy(),
    )


def first_preset_id_or_skip(exasol_path: str, preset_type: str) -> str:
    """Return the first embedded preset ID for a given type, or skip if none exist."""
    result = run_command([exasol_path, "presets", "list", "--json"])
    data = json.loads(result.stdout)
    presets_list = data.get(preset_type)
    if not isinstance(presets_list, list) or len(presets_list) == 0:
        pytest.skip(f"no presets found for type {preset_type!r}")

    preset_id = presets_list[0].get("id")
    if not isinstance(preset_id, str) or preset_id.strip() == "":
        pytest.skip(f"first preset in type {preset_type!r} has no id")

    return preset_id


def first_infrastructure_preset_id_or_skip(exasol_path: str) -> str:
    return first_preset_id_or_skip(exasol_path, "infrastructures")


def first_installation_preset_id_or_skip(exasol_path: str) -> str:
    return first_preset_id_or_skip(exasol_path, "installations")


def export_preset(
    exasol_path: str, preset_id: str, preset_type: str, to_dir: str
) -> None:
    """Export a preset to a directory (used to test preset path argument variants)."""
    run_command(
        [
            exasol_path,
            "presets",
            "export",
            preset_id,
            "--type",
            preset_type,
            "--to",
            to_dir,
        ]
    )
