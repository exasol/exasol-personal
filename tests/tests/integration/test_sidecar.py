# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

import json
from pathlib import Path
from subprocess import CalledProcessError

import pytest

from .helpers import run_command


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


def test_sidecar_list_reports_availability(
    exasol_path: str, sidecar_directory: Path
) -> None:
    # Given
    selection = ["--deployment-dir", str(sidecar_directory)]
    entries = json.loads(
        run_command([exasol_path, "sidecar", "list", "--json", *selection]).stdout
    )
    supported = [entry for entry in entries if entry["supported"]]
    if not supported:
        pytest.skip("requires the sidecar fixture catalog build")
    name = supported[0]["name"]
    run_command([exasol_path, "sidecar", "enable", name, *selection])
    # When
    listing = json.loads(
        run_command([exasol_path, "sidecar", "list", "--json", *selection]).stdout
    )
    text = run_command([exasol_path, "sidecar", "list", *selection]).stdout
    # Then
    selected = next(entry for entry in listing if entry["name"] == name)
    assert selected["enabled"]
    assert selected["architectures"]
    assert selected["description"]
    assert name in text
    assert "enabled" in text
    assert selected["description"] in text
    assert all(architecture in text for architecture in selected["architectures"])


@pytest.mark.parametrize("choice", ["unknown", "incompatible"])
def test_sidecar_enable_reports_choices(
    exasol_path: str, sidecar_directory: Path, choice: str
) -> None:
    # Given
    selection = ["--deployment-dir", str(sidecar_directory)]
    name = "unknown-sidecar"
    if choice == "incompatible":
        entries = json.loads(
            run_command([exasol_path, "sidecar", "list", "--json", *selection]).stdout
        )
        incompatible = [entry for entry in entries if not entry["supported"]]
        if not incompatible:
            pytest.skip("requires an architecture-specific fixture entry")
        name = incompatible[0]["name"]
    # When
    with pytest.raises(CalledProcessError) as failure:
        run_command([exasol_path, "sidecar", "enable", name, *selection])
    # Then
    assert "supported choices:" in failure.value.stderr
    assert name in failure.value.stderr
    assert not (sidecar_directory / "sidecars.yaml").exists()


def test_sidecar_cloud_enable_before_provisioning(
    exasol_path: str, tmp_path: Path
) -> None:
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
    (directory / "sidecars.yaml").write_text(
        "version: 1\ncontainers:\n- name: example\n  image: caddy:2\n", encoding="utf-8"
    )
    # When
    result = json.loads(
        run_command(
            [exasol_path, "sidecar", "enable", "example", "--json", *selection]
        ).stdout
    )
    # Then
    assert result == {"name": "example", "enabled": True, "hosts": []}


def test_sidecar_saved_definition_commands(
    exasol_path: str, sidecar_directory: Path
) -> None:
    # Given
    definition = sidecar_directory / "sidecars.yaml"
    definition.write_text(
        "version: 1\ncontainers:\n- name: saved\n  image: caddy:2\n", encoding="utf-8"
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
    again = json.loads(
        run_command([exasol_path, "sidecar", "disable", "saved", *selection]).stdout
    )
    # Then
    assert enabled == status
    assert enabled["enabled"]
    assert not enabled["hosts"][0]["running"]
    assert enabled["hosts"][0]["endpoints"] == []
    assert disabled == again
    assert not disabled["enabled"]
    assert not disabled["hosts"][0]["running"]
    log = (sidecar_directory / "deployment.log").read_text(encoding="utf-8")
    for operation in ["enable", "status", "disable"]:
        assert f"command={operation}" in log


@pytest.mark.parametrize("operation", ["enable", "status", "disable"])
def test_sidecar_name_argument_is_required(exasol_path: str, operation: str) -> None:
    # When
    with pytest.raises(CalledProcessError) as failure:
        run_command([exasol_path, "sidecar", operation])
    # Then
    assert "accepts 1 arg(s)" in failure.value.stderr
