# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

import json
from pathlib import Path
from subprocess import CalledProcessError
from typing import Any

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


def test_sidecar_removal_returns_catalog_sidecar_to_template(
    exasol_path: str, sidecar_directory: Path
) -> None:
    # Given
    definition = sidecar_directory / "sidecars.yaml"
    selection = ["--deployment-dir", str(sidecar_directory), "--json"]
    run_command([exasol_path, "sidecar", "enable", "mcp", *selection])
    template = yaml.safe_load(definition.read_text(encoding="utf-8"))
    edited = yaml.safe_load(definition.read_text(encoding="utf-8"))
    edited["sidecars"]["mcp"]["container"]["ports"][0]["hostPort"] = 14896
    definition.write_text(yaml.safe_dump(edited), encoding="utf-8")
    # When
    run_command([exasol_path, "sidecar", "disable", "mcp", "--remove", *selection])
    removed = yaml.safe_load(definition.read_text(encoding="utf-8"))
    run_command([exasol_path, "sidecar", "enable", "mcp", *selection])
    restored = yaml.safe_load(definition.read_text(encoding="utf-8"))
    # Then
    assert "mcp" not in removed["sidecars"]
    assert restored == template


def test_sidecar_repeated_removal_reports_nothing_to_remove(
    exasol_path: str, sidecar_directory: Path
) -> None:
    # Given
    (sidecar_directory / "sidecars.yaml").write_text(
        "version: 1\nsidecars:\n  saved:\n    container:\n      image: caddy:2\n",
        encoding="utf-8",
    )
    command = [
        exasol_path,
        "sidecar",
        "disable",
        "saved",
        "--remove",
        "--deployment-dir",
        str(sidecar_directory),
    ]
    run_command(command)
    # When
    repeated = run_command(command)
    # Then
    assert "nothing to remove" in repeated.stdout


def test_sidecar_enable_saves_declared_probes_with_defaults(
    exasol_path: str, sidecar_directory: Path
) -> None:
    # Given
    selection = ["--deployment-dir", str(sidecar_directory), "--json"]
    # When
    run_command([exasol_path, "sidecar", "enable", "mcp", *selection])
    # Then
    saved = yaml.safe_load(
        (sidecar_directory / "sidecars.yaml").read_text(encoding="utf-8")
    )
    assert saved["sidecars"]["mcp"]["container"]["readinessProbe"] == {
        "httpGet": {"path": "/health", "port": "mcp", "scheme": "HTTP"},
        "periodSeconds": 10,
        "timeoutSeconds": 1,
        "failureThreshold": 6,
    }


def test_sidecar_invalid_probe_names_the_offending_field(
    exasol_path: str, sidecar_directory: Path
) -> None:
    # Given
    (sidecar_directory / "sidecars.yaml").write_text(
        "version: 1\nsidecars:\n  saved:\n    container:\n      image: caddy:2\n"
        "      readinessProbe: {httpGet: {port: 8080}}\n",
        encoding="utf-8",
    )
    # When
    with pytest.raises(CalledProcessError) as failure:
        run_command(
            [
                exasol_path,
                "sidecar",
                "status",
                "saved",
                "--deployment-dir",
                str(sidecar_directory),
            ]
        )
    # Then
    assert "sidecars.saved.container.readinessProbe.httpGet.port" in (
        failure.value.stderr
    )


def _saved_mcp(directory: Path) -> dict[str, Any]:
    saved = yaml.safe_load((directory / "sidecars.yaml").read_text(encoding="utf-8"))
    return dict(saved["sidecars"]["mcp"])


def test_sidecar_enable_saves_explicit_host_port(
    exasol_path: str, sidecar_directory: Path
) -> None:
    # Given
    selection = ["--deployment-dir", str(sidecar_directory)]
    # When
    run_command(
        [exasol_path, "sidecar", "enable", "mcp", "--ports", "mcp:14897", *selection]
    )
    # Then
    assert _saved_mcp(sidecar_directory)["container"]["ports"][0]["hostPort"] == 14897  # noqa: PLR2004


def test_sidecar_enable_with_ports_updates_enabled_sidecar(
    exasol_path: str, sidecar_directory: Path
) -> None:
    # Given
    selection = ["--deployment-dir", str(sidecar_directory)]
    run_command(
        [exasol_path, "sidecar", "enable", "mcp", "--ports", "mcp:14897", *selection]
    )
    before = _saved_mcp(sidecar_directory)
    # When
    run_command(
        [exasol_path, "sidecar", "enable", "mcp", "--ports", "mcp:14898", *selection]
    )
    # Then
    after = _saved_mcp(sidecar_directory)
    assert after["container"]["ports"][0]["hostPort"] == 14898  # noqa: PLR2004
    after["container"]["ports"][0]["hostPort"] = 14897
    assert after == before


@pytest.mark.parametrize(
    "entry",
    [
        ":14897",
        "unknown:14897",
        "internal:14897",
        "mcp:14897,mcp:14898",
        "mcp:65536",
        "mcp:18563",
        "mcp:18080",
    ],
)
def test_sidecar_invalid_port_entry_names_the_entry(
    exasol_path: str, tmp_path: Path, entry: str
) -> None:
    # Given
    directory = tmp_path / "deployment"
    selection = ["--deployment-dir", str(directory)]
    run_command(
        [
            exasol_path,
            "init",
            "local",
            "--ports",
            "db:18563",
            *selection,
            "--no-launcher-version-check",
        ]
    )
    definition = directory / "sidecars.yaml"
    definition.write_text(
        "version: 1\nsidecars:\n"
        "  mcp:\n    container:\n      image: caddy:2\n      ports:\n"
        "        - {name: mcp, containerPort: 4896, hostPort: 14896}\n"
        "        - {name: internal, containerPort: 9090}\n"
        "  other:\n    container:\n      image: caddy:2\n      ports:\n"
        "        - {name: http, containerPort: 8080, hostPort: 18080}\n",
        encoding="utf-8",
    )
    saved = definition.read_text(encoding="utf-8")
    offending = entry.rsplit(",", maxsplit=1)[-1]
    # When
    with pytest.raises(CalledProcessError) as failure:
        run_command(
            [exasol_path, "sidecar", "enable", "mcp", "--ports", entry, *selection]
        )
    # Then
    assert f'"{offending}"' in failure.value.stderr
    assert definition.read_text(encoding="utf-8") == saved


@pytest.mark.openspec("deployment-info-reporting")
def test_info_reports_saved_sidecars(exasol_path: str, sidecar_directory: Path) -> None:
    # Given
    (sidecar_directory / "sidecars.yaml").write_text(
        "version: 1\nsidecars:\n"
        "  api:\n    container:\n      image: caddy:2\n"
        "      ports:\n        - name: http\n          containerPort: 8080\n"
        "          hostPort: 18080\n"
        "    endpoints:\n      - port: http\n        scheme: http\n        path: /v1\n"
        "  idle:\n    enabled: false\n    container:\n      image: caddy:2\n",
        encoding="utf-8",
    )
    selection = ["--deployment-dir", str(sidecar_directory)]
    # When
    text = run_command([exasol_path, "info", *selection]).stdout
    data = json.loads(run_command([exasol_path, "info", *selection, "--json"]).stdout)
    # Then
    assert "  api: enabled http://127.0.0.1:18080/v1" in text
    assert "  idle: disabled" in text
    assert data["sidecars"] == [
        {
            "name": "api",
            "enabled": True,
            "endpoints": [
                {"ip": "127.0.0.1", "port": 18080, "url": "http://127.0.0.1:18080/v1"}
            ],
        },
        {"name": "idle", "enabled": False, "endpoints": []},
    ]


@pytest.mark.openspec("deployment-info-reporting")
def test_info_reports_no_saved_sidecars(
    exasol_path: str, sidecar_directory: Path
) -> None:
    # When
    data = json.loads(
        run_command(
            [exasol_path, "info", "--deployment-dir", str(sidecar_directory), "--json"]
        ).stdout
    )
    # Then
    assert data["sidecars"] == []
