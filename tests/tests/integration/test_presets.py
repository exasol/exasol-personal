# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

import json
from pathlib import Path
from subprocess import CalledProcessError
from typing import Any

import pytest

from .helpers import run_command


def _get_presets_catalog(exasol_path: str) -> Any:  # noqa: ANN401
    result = run_command([exasol_path, "presets", "list", "--json"])
    return json.loads(result.stdout)


def _first_preset_id_or_skip(exasol_path: str, preset_type: str) -> str:
    catalog = _get_presets_catalog(exasol_path)
    presets_list = catalog.get(preset_type)
    if not isinstance(presets_list, list) or len(presets_list) == 0:
        pytest.skip(f"no presets found for type {preset_type!r}")

    preset_id = presets_list[0].get("id")
    if not isinstance(preset_id, str) or preset_id.strip() == "":
        pytest.skip(f"first preset in type {preset_type!r} has no id")

    return preset_id


def test_presets_help_mentions_subcommands(exasol_path: str) -> None:
    # When I call `exasol presets --help`
    result = run_command([exasol_path, "presets", "--help"])
    output: str = result.stdout

    # Then I see that this command group exists and offers basic functionality
    assert "presets" in output.lower()
    assert "list" in output.lower()
    assert "export" in output.lower()


def test_presets_list_outputs_sections(exasol_path: str) -> None:
    # When I call `exasol presets list`
    result = run_command([exasol_path, "presets", "list"])
    output: str = result.stdout

    # Then I see headers for the known preset types
    assert "Infrastructure presets:" in output
    assert "Installation presets:" in output


def test_presets_list_json_is_valid(exasol_path: str) -> None:
    # When I call `exasol presets list --json`
    result = run_command([exasol_path, "presets", "list", "--json"])

    # Then the output is parseable JSON with the expected top-level keys
    data = json.loads(result.stdout)
    assert isinstance(data, dict)
    assert "infrastructures" in data
    assert "installations" in data


def test_presets_export_writes_files(exasol_path: str, tmp_path: Path) -> None:
    # Given an empty target directory for exporting an infrastructure preset
    infra_id = _first_preset_id_or_skip(exasol_path, "infrastructures")
    infra_dir = tmp_path / "infra_export"
    infra_dir.mkdir()

    # When `exasol presets export` is called for that infrastructure preset
    result_infra = run_command(
        [
            exasol_path,
            "presets",
            "export",
            infra_id,
            "--type",
            "infrastructure",
            "--to",
            str(infra_dir),
        ]
    )

    # Then it succeeds and writes some files
    assert result_infra.returncode == 0
    assert any(infra_dir.iterdir())
    assert (infra_dir / "infrastructure.yaml").exists()

    # Given an empty target directory for exporting an installation preset
    install_id = _first_preset_id_or_skip(exasol_path, "installations")
    install_dir = tmp_path / "install_export"
    install_dir.mkdir()

    # When `exasol presets export` is called for that installation preset
    result_install = run_command(
        [
            exasol_path,
            "presets",
            "export",
            install_id,
            "--type",
            "installation",
            "--to",
            str(install_dir),
        ]
    )

    # Then it succeeds and writes some files
    assert result_install.returncode == 0
    assert any(install_dir.iterdir())
    assert (install_dir / "installation.yaml").exists()


def test_presets_export_fails_on_non_empty_dir(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given a non-empty target directory
    preset_id = _first_preset_id_or_skip(exasol_path, "infrastructures")
    target = tmp_path / "non_empty"
    target.mkdir()
    (target / "somefile.txt").write_text("x")

    # When `exasol presets export` is called
    with pytest.raises(CalledProcessError) as exc:
        run_command(
            [
                exasol_path,
                "presets",
                "export",
                preset_id,
                "--type",
                "infrastructure",
                "--to",
                str(target),
            ]
        )

    # Then it fails
    assert exc.value.returncode != 0
    assert "not empty" in exc.value.stderr.lower()
