# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

import json
from pathlib import Path
from subprocess import CalledProcessError

import pytest
import yaml

from .helpers import run_command

pytestmark = pytest.mark.openspec("sidecar-configuration", "sidecar-lifecycle")


@pytest.fixture
def sidecar_directory(exasol_path: str, tmp_path: Path) -> Path:
    directory = tmp_path / "deployment"
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
    return directory


def test_sidecar_unsupported_deployment_type(exasol_path: str, tmp_path: Path) -> None:
    # Given
    directory = tmp_path / "cloud"
    selection = ["--deployment-dir", str(directory)]
    run_command(
        [
            exasol_path,
            "init",
            "aws",
            "ubuntu",
            *selection,
            "--no-launcher-version-check",
        ]
    )
    definition = (
        "version: 1\nsidecars:\n  example:\n    container:\n      image: caddy:2\n"
    )
    (directory / "sidecars.yaml").write_text(definition, encoding="utf-8")
    # When
    with pytest.raises(CalledProcessError) as failure:
        run_command([exasol_path, "sidecar", "enable", "example", *selection])
    # Then
    assert "not supported for this deployment type" in failure.value.stderr
    assert (directory / "sidecars.yaml").read_text(encoding="utf-8") == definition


def test_sidecar_saved_definition_commands(
    exasol_path: str, sidecar_directory: Path
) -> None:
    # Given
    definition = sidecar_directory / "sidecars.yaml"
    definition.write_text(
        "version: 1\nsidecars:\n  saved:\n    container:\n      image: caddy:2\n",
        encoding="utf-8",
    )
    selection = ["--deployment-dir", str(sidecar_directory), "--json"]
    # When
    enabled = json.loads(
        run_command([exasol_path, "sidecar", "enable", "saved", *selection]).stdout
    )
    status = json.loads(
        run_command([exasol_path, "sidecar", "status", "saved", *selection]).stdout
    )
    disabled = json.loads(
        run_command([exasol_path, "sidecar", "disable", "saved", *selection]).stdout
    )
    repeated = json.loads(
        run_command([exasol_path, "sidecar", "disable", "saved", *selection]).stdout
    )
    # Then
    assert enabled == status
    assert enabled["enabled"]
    assert not enabled["running"]
    assert enabled["endpoints"] == []
    assert not disabled["enabled"]
    assert not disabled["running"]
    assert repeated["nothingToRemove"] is True
    assert not repeated["enabled"]
    saved = yaml.safe_load(definition.read_text(encoding="utf-8"))["sidecars"]
    assert saved["saved"]["enabled"] is False
    assert saved["saved"]["container"]["image"] == "caddy:2"
    log = (sidecar_directory / "deployment.log").read_text(encoding="utf-8")
    for operation in ["enable", "status", "disable"]:
        assert f"command={operation}" in log


@pytest.mark.parametrize("operation", ["enable", "disable"])
def test_sidecar_name_argument_is_required(exasol_path: str, operation: str) -> None:
    # When
    with pytest.raises(CalledProcessError) as failure:
        run_command([exasol_path, "sidecar", operation])
    # Then
    assert "accepts 1 arg(s)" in failure.value.stderr


def test_sidecar_status_without_a_name_reports_every_enabled_sidecar(
    exasol_path: str, sidecar_directory: Path
) -> None:
    # Given
    definition = sidecar_directory / "sidecars.yaml"
    definition.write_text(
        "version: 1\nsidecars:\n"
        "  second:\n    container:\n      image: caddy:2\n"
        "  first:\n    container:\n      image: caddy:2\n",
        encoding="utf-8",
    )
    selection = ["--deployment-dir", str(sidecar_directory), "--json"]
    # When
    reported = json.loads(
        run_command([exasol_path, "sidecar", "status", *selection]).stdout
    )
    # Then
    assert [entry["name"] for entry in reported] == ["first", "second"]
    assert all(entry["enabled"] for entry in reported)


def _reported_enablement(
    exasol_path: str, directory: Path
) -> tuple[bool, bool, list[str]]:
    selection = ["--deployment-dir", str(directory), "--json"]
    listed = json.loads(
        run_command([exasol_path, "sidecar", "list", *selection]).stdout
    )
    status = json.loads(
        run_command([exasol_path, "sidecar", "status", "mcp", *selection]).stdout
    )
    every = json.loads(
        run_command([exasol_path, "sidecar", "status", *selection]).stdout
    )
    mcp = next(entry for entry in listed if entry["name"] == "mcp")
    return mcp["enabled"], status["enabled"], [entry["name"] for entry in every]


def test_sidecar_entry_without_enabled_field_is_enabled(
    exasol_path: str, sidecar_directory: Path
) -> None:
    # Given
    (sidecar_directory / "sidecars.yaml").write_text(
        "version: 1\nsidecars:\n  mcp:\n    container:\n      image: caddy:2\n",
        encoding="utf-8",
    )
    # When
    listed, status, _ = _reported_enablement(exasol_path, sidecar_directory)
    # Then
    assert listed is True
    assert status is True


def test_sidecar_disabled_entry_reports_disabled(
    exasol_path: str, sidecar_directory: Path
) -> None:
    # Given
    (sidecar_directory / "sidecars.yaml").write_text(
        "version: 1\nsidecars:\n"
        "  mcp:\n    enabled: false\n    container:\n      image: caddy:2\n",
        encoding="utf-8",
    )
    # When
    listed, status, every = _reported_enablement(exasol_path, sidecar_directory)
    # Then
    assert listed is False
    assert status is False
    assert every == ["mcp"]
