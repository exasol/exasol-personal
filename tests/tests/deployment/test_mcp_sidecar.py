# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

import asyncio
import json
import socket
import time
from collections.abc import Iterator
from typing import Any

import pytest
import requests
from mcp import Client
from mcp.types import CallToolResult, TextContent

from framework.sidecars import SidecarDeployment, sidecar_deployment

pytestmark = pytest.mark.local_e2e

MCP_SIDECAR = "mcp"
MCP_HEALTH_TIMEOUT = 60
MCP_READ_QUERY_TOOL = "execute_exasol_query"


@pytest.fixture
def mcp_deployment(exasol_path: str, infra: str) -> Iterator[SidecarDeployment]:
    with sidecar_deployment(
        exasol_path,
        infra,
        running=False,
        sidecar_name=MCP_SIDECAR,
    ) as service:
        yield service


def wait_until_healthy(endpoint: str) -> dict[str, Any]:
    deadline = time.monotonic() + MCP_HEALTH_TIMEOUT
    last_observation = "no response"
    with requests.Session() as session:
        session.trust_env = False
        while time.monotonic() < deadline:
            try:
                response = session.get(f"{endpoint}/health", timeout=2)
                response.raise_for_status()
                health = dict(response.json())
                last_observation = json.dumps(health, sort_keys=True)
                if health.get("status") == "healthy":
                    return health
            except (requests.RequestException, ValueError) as error:
                last_observation = str(error)
            time.sleep(0.2)
    pytest.fail(
        f"MCP sidecar did not become healthy within {MCP_HEALTH_TIMEOUT} seconds; "
        f"last observation: {last_observation}"
    )


def available_loopback_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as listener:
        listener.bind(("127.0.0.1", 0))
        return int(listener.getsockname()[1])


def set_host_port(service: SidecarDeployment, port: int) -> None:
    definition = service.definition()
    definition["ports"][0]["hostPort"] = port
    service.save(definition)


async def execute_test_query(endpoint: str) -> tuple[set[str], CallToolResult]:
    async with Client(f"{endpoint}/mcp") as client:
        tools = await client.list_tools()
        result = await client.call_tool(MCP_READ_QUERY_TOOL, {"query": "SELECT 42"})
    return {tool.name for tool in tools.tools}, result


def test_mcp_sidecar_connects_to_local_database(
    mcp_deployment: SidecarDeployment,
) -> None:
    # Given
    host_port = available_loopback_port()
    mcp_deployment.sidecar("enable")
    set_host_port(mcp_deployment, host_port)
    # When
    mcp_deployment.deployment.deploy()
    status = mcp_deployment.sidecar("status")
    hosts = list(status["hosts"])
    assert len(hosts) == 1
    host = dict(hosts[0])
    endpoint = dict(host["endpoints"][0])
    endpoint_url = f"http://{endpoint['ip']}:{endpoint['port']}"
    health = wait_until_healthy(endpoint_url)
    tool_names, query_result = asyncio.run(execute_test_query(endpoint_url))
    # Then
    assert status["enabled"] is True
    assert host["running"] is True
    assert host["reconciliation"] == "complete"
    assert host["endpoints"] == [{"ip": "127.0.0.1", "port": host_port}]
    assert health["service"] == "exasol-mcp-server"
    assert MCP_READ_QUERY_TOOL in tool_names
    assert "execute_exasol_write_query" not in tool_names
    assert "set_exasol_preprocessor" not in tool_names
    assert not any("bucketfs" in name for name in tool_names)
    assert query_result.is_error is False
    query_text = "\n".join(
        block.text for block in query_result.content if isinstance(block, TextContent)
    )
    assert "42" in query_text
