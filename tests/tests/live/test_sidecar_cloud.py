# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

import json
from pathlib import Path
from typing import Any

import pytest
import yaml

from framework.deployment import Deployment, StatusDatabaseReady
from framework.outputs import get_outputs
from framework.sidecars import SidecarDeployment

pytestmark = [
    pytest.mark.openspec(
        "sidecar-configuration", "sidecar-lifecycle", "sidecar-networking"
    ),
    pytest.mark.providers("aws", "azure", "exoscale"),
]


def _cloud_definition(listen: str = "127.0.0.1:8080") -> dict[str, Any]:
    catalog = yaml.safe_load(
        (Path(__file__).parents[2] / "fixtures/sidecars/catalog.yaml").read_text()
    )
    definition = catalog["sidecars"]["caddy"]["container"]
    definition["name"] = "caddy"
    definition["ports"][0]["hostPort"] = 8080
    definition["args"][2] = listen
    return dict(definition)


def test_sidecar_cloud_nodes_reconcile(reusable_live_deployment: Deployment) -> None:
    # Given
    deployment = reusable_live_deployment
    service = SidecarDeployment(deployment)
    nodes = get_outputs(str(service.directory)).nodes
    definition = _cloud_definition()
    password = json.loads((service.directory / "secrets.json").read_text())[
        "dbPassword"
    ]
    service.save(definition)
    try:
        # When
        enabled = service.sidecar("enable")
        # Then
        assert enabled["enabled"]
        assert {host["name"] for host in enabled["hosts"]} == set(nodes)
        for host in enabled["hosts"]:
            node = host["name"]
            assert host["running"]
            assert host["reconciliation"] == "complete"
            assert host["endpoints"] == [{"ip": "127.0.0.1", "port": 8080}]
            endpoint = host["endpoints"][0]
            assert (
                service.internal_response(
                    f"http://{endpoint['ip']}:{endpoint['port']}", node=node
                )
                == f"database|8563|sys|{password}|initial-marker"
            )
            assert service.inspect(node=node)["HostConfig"]["NetworkMode"] == "host"
            hosts = service.podman(
                "exec", service.container, "cat", "/etc/hosts", node=node
            )
            assert any(
                line.split()[0] == nodes[node].privateIp
                and "database" in line.split()[1:]
                for line in hosts.splitlines()
                if line.split()
            )
            service.podman(
                "exec",
                service.container,
                "sh",
                "-c",
                "nc -w 2 database 8563 </dev/null",
                node=node,
            )
        # When
        next(v for v in definition["env"] if v["name"] == "TEST_MARKER")["value"] = (
            "edited"
        )
        service.save(definition)
        service.podman("rm", "--force", service.container, node=next(iter(nodes)))
        deployment.start()
        # Then
        for node in nodes:
            assert (
                service.internal_response("http://127.0.0.1:8080", node=node)
                == f"database|8563|sys|{password}|edited"
            )
        assert deployment.status_value() == StatusDatabaseReady
        assert "42" in service.sql()
    finally:
        service.sidecar("disable")
    # Then
    disabled = service.sidecar("status")
    assert not disabled["enabled"]
    assert {host["name"] for host in disabled["hosts"]} == set(nodes)
    for host in disabled["hosts"]:
        assert not host["running"]
        assert host["endpoints"] == []
        assert host["reconciliation"] == "complete"
    assert "42" in service.sql()


def test_sidecar_cloud_declared_endpoint_admitted(
    reusable_live_deployment: Deployment,
) -> None:
    # Given
    deployment = reusable_live_deployment
    service = SidecarDeployment(deployment)
    nodes = get_outputs(str(service.directory)).nodes
    service.save(_cloud_definition(listen=":8080"))
    endpoints = [f"http://{node.publicIp}:8080" for node in nodes.values()]
    try:
        # When
        service.sidecar("enable")
        # Then
        for endpoint in endpoints:
            assert "initial-marker" in service.external_response(endpoint, timeout=120)
        # When
        deployment.stop()
        deployment.start()
        # Then
        for endpoint in endpoints:
            assert "initial-marker" in service.external_response(endpoint, timeout=180)
        assert deployment.status_value() == StatusDatabaseReady
    finally:
        service.sidecar("disable")
    # Then
    for endpoint in endpoints:
        service.external_unreachable(endpoint)
    assert "42" in service.sql()


def test_sidecar_cloud_service_restart(reusable_live_deployment: Deployment) -> None:
    # Given
    deployment = reusable_live_deployment
    service = SidecarDeployment(deployment)
    node = next(iter(get_outputs(str(service.directory)).nodes))
    service.save(_cloud_definition())
    try:
        service.sidecar("enable")
        assert "PartOf=c4_cloud_command.service" in service.unit("cat", node=node)
        # When
        service.systemd("restart", "c4_cloud_command.service", node=node)
        # Then
        assert service.unit_active(node=node)
        assert "initial-marker" in service.internal_response(
            "http://127.0.0.1:8080", node=node
        )
        # When
        service.systemd("stop", "c4_cloud_command.service", node=node)
        # Then
        assert not service.unit_active(node=node)
    finally:
        service.systemd("start", "c4_cloud_command.service", node=node)
        service.sidecar("disable")
    assert "42" in service.sql()
