# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

"""Public endpoint reporting through the CLI, without provisioned infrastructure."""

import json
import subprocess
from pathlib import Path

import pytest
import yaml

from .helpers import run_command

pytestmark = pytest.mark.openspec("deployment-info-reporting")
DB_PORT = 8563


@pytest.mark.parametrize("all_nodes", [False, True])
def test_info_reports_public_endpoints_and_eligibility(
    exasol_path: str, tmp_path: Path, *, all_nodes: bool
) -> None:
    # Given a saved cloud deployment with two public nodes and one private node.
    deployment = _saved_cloud_deployment(exasol_path, tmp_path, all_nodes=all_nodes)

    # When info is requested in text and JSON form.
    command = [exasol_path, "info", "--deployment-dir", str(deployment)]
    text = run_command(command)
    encoded = run_command([*command, "--json"])

    # Then both report public endpoints, preserving the existing primary fields.
    connection = json.loads(encoded.stdout)["connection"]
    assert connection["host"] == "primary.example"
    assert connection["dbPort"] == DB_PORT
    assert connection["endpoints"] == [
        {
            "node": "n11",
            "host": "203.0.113.11",
            "dbPort": 8563,
            "eligible": True,
        },
        {
            "node": "n12",
            "host": "203.0.113.12",
            "dbPort": 8563,
            "eligible": all_nodes,
        },
    ]
    assert "saved eligibility; reachability not checked" in text.stdout
    eligibility = "eligible" if all_nodes else "topology only"
    assert "n11: 203.0.113.11:8563 (eligible)" in text.stdout
    assert f"n12: 203.0.113.12:8563 ({eligibility})" in text.stdout
    assert "10.0.0.13" not in text.stdout


def _saved_cloud_deployment(
    exasol_path: str, tmp_path: Path, *, all_nodes: bool
) -> Path:
    # Given a saved cloud deployment with two public nodes and one private node.
    deployment = tmp_path / "deployment"
    run_command(
        [
            exasol_path,
            "init",
            "aws",
            "--deployment-dir",
            str(deployment),
            "--no-launcher-version-check",
        ]
    )
    manifest = deployment / "installation/installation.yaml"
    saved = yaml.safe_load(manifest.read_text())
    if all_nodes:
        saved["databaseConnection"] = {"allowAllNodes": True}
    else:
        saved.pop("databaseConnection", None)
    manifest.write_text(yaml.safe_dump(saved))
    state_file = deployment / ".exasolLauncherState.json"
    state = json.loads(state_file.read_text())
    state["currentWorkflowState"] = {"running": {}}
    state_file.write_text(json.dumps(state))
    (deployment / "secrets.json").write_text(json.dumps({"dbPassword": "example"}))
    (deployment / "deployment.json").write_text(
        json.dumps(
            {
                "backend": "tofu",
                "connection": {"host": "primary.example", "dbPort": 8563},
                "nodes": {
                    "n12": {
                        "publicIp": "203.0.113.12",
                        "database": {"dbPort": "8563"},
                    },
                    "n11": {
                        "publicIp": "203.0.113.11",
                        "dnsName": "primary.example",
                        "database": {"dbPort": "8563"},
                    },
                    "n13": {
                        "privateIp": "10.0.0.13",
                        "database": {"dbPort": "8563"},
                    },
                },
            }
        )
    )

    return deployment


@pytest.mark.parametrize("expose_secondary", [False, True])
@pytest.mark.openspec("multi-node-connections")
def test_info_combines_login_permission_with_infrastructure_exposure(
    exasol_path: str, tmp_path: Path, *, expose_secondary: bool
) -> None:
    # Given a cloud installation permitting all nodes, with no public primary endpoint.
    deployment = _saved_cloud_deployment(exasol_path, tmp_path, all_nodes=True)
    output_file = deployment / "deployment.json"
    output = json.loads(output_file.read_text())
    output["connection"] = {"host": "private-primary.example", "dbPort": 8563}
    for name, node in output["nodes"].items():
        node.pop("publicIp", None)
        node["privateIp"] = "10.0.0." + name[1:]
    if expose_secondary:
        output["nodes"]["n12"]["publicIp"] = "203.0.113.12"
        output["nodes"]["n12"]["database"]["dbPort"] = "9000"
    output_file.write_text(json.dumps(output))

    # When requesting connection information.
    command = [exasol_path, "info", "--deployment-dir", str(deployment)]
    text = run_command(command)
    report = json.loads(run_command([*command, "--json"]).stdout)

    # Then eligibility reflects only the infrastructure's published database endpoints.
    connection = report["connection"]
    assert connection["host"] == "private-primary.example"
    assert connection["dbPort"] == DB_PORT
    expected = (
        [{"node": "n12", "host": "203.0.113.12", "dbPort": 9000, "eligible": True}]
        if expose_secondary
        else []
    )
    assert connection["endpoints"] == expected
    assert "private-primary.example:8563 (eligible)" not in text.stdout
    if not expose_secondary:
        assert "No public database endpoints" in text.stdout
        failed = subprocess.run(
            [
                exasol_path,
                "connect",
                "--deployment-dir",
                str(deployment),
                "-c",
                "SELECT 1",
            ],
            check=False,
            text=True,
            capture_output=True,
        )
        assert failed.returncode != 0
        assert "no eligible public database endpoints" in failed.stderr
