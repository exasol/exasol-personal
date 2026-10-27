# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

import asyncio
import json
import socket
import time
from collections.abc import Iterator
from http import HTTPStatus
from typing import Any
from urllib.parse import urljoin

import pytest
import requests
from mcp import Client
from mcp.types import CallToolResult, TextContent

from framework.deployment import DeploymentManager
from framework.sidecars import SidecarDeployment, sidecar_deployment

pytestmark = [
    pytest.mark.openspec(
        "sidecar-configuration", "sidecar-lifecycle", "sidecar-networking"
    ),
    pytest.mark.providers("local"),
    pytest.mark.platform("linux-amd64", "macos-arm64", "windows-amd64"),
]

MCP_SIDECAR = "mcp"
MCP_HEALTH_TIMEOUT = 60
MCP_READ_QUERY_TOOL = "execute_exasol_query"
MCP_METADATA_TOOL = "list_exasol_tables_and_views"
MCP_TEST_SCHEMA = "MCP_SIDECAR_TEST"
MCP_TEST_VIEW = "ACTIVE_USER"
MCP_TEST_VALUE = "MCP_DATA_SURVIVES"
MCP_TEST_QUERY = "SELECT USER_NAME FROM MCP_SIDECAR_TEST.ACTIVE_USER"
MCP_ROW_LIMIT = 1000
MCP_ROW_LIMIT_QUERY = "SELECT LEVEL AS ID FROM DUAL CONNECT BY LEVEL <= 1001"
MCP_TEST_METADATA_SQL = """
CREATE SCHEMA MCP_SIDECAR_TEST;
CREATE TABLE MCP_SIDECAR_TEST.USERS (USER_NAME VARCHAR(128));
INSERT INTO MCP_SIDECAR_TEST.USERS VALUES ('MCP_DATA_SURVIVES');
CREATE VIEW MCP_SIDECAR_TEST.ACTIVE_USER AS
SELECT USER_NAME FROM MCP_SIDECAR_TEST.USERS;
COMMIT;
"""


@pytest.fixture
def mcp_deployment(
    deployment_manager: DeploymentManager, infra: str
) -> Iterator[SidecarDeployment]:
    with sidecar_deployment(
        deployment_manager,
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
                response = session.get(urljoin(endpoint, "/health"), timeout=2)
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


async def exercise_mcp(
    endpoint: str,
) -> tuple[set[str], CallToolResult, CallToolResult, CallToolResult]:
    async with Client(endpoint) as client:
        tools = await client.list_tools()
        metadata_result = await client.call_tool(
            MCP_METADATA_TOOL, {"schema_name": MCP_TEST_SCHEMA}
        )
        query_result = await client.call_tool(
            MCP_READ_QUERY_TOOL, {"query": MCP_TEST_QUERY}
        )
        row_limit_result = await client.call_tool(
            MCP_READ_QUERY_TOOL, {"query": MCP_ROW_LIMIT_QUERY}
        )
    return (
        {tool.name for tool in tools.tools},
        metadata_result,
        query_result,
        row_limit_result,
    )


def result_text(result: CallToolResult) -> str:
    return "\n".join(
        block.text for block in result.content if isinstance(block, TextContent)
    )


def running_endpoint(status: dict[str, Any], host_port: int) -> str:
    """Return the URL status reports, so callers connect to the reported value."""
    assert status["enabled"] is True
    assert status["running"] is True
    assert status["reconciliation"] == "complete"
    assert status["endpoints"] == [
        {
            "ip": "127.0.0.1",
            "port": host_port,
            "url": f"http://127.0.0.1:{host_port}/mcp",
        }
    ]
    return str(status["endpoints"][0]["url"])


def assert_mcp_interaction(endpoint: str) -> None:
    health = wait_until_healthy(endpoint)
    tool_names, metadata_result, query_result, row_limit_result = asyncio.run(
        exercise_mcp(endpoint)
    )

    assert health["service"] == "exasol-mcp-server"
    assert MCP_READ_QUERY_TOOL in tool_names
    assert MCP_METADATA_TOOL in tool_names
    assert "execute_exasol_write_query" not in tool_names
    assert "set_exasol_preprocessor" not in tool_names
    assert not any("bucketfs" in name for name in tool_names)
    assert metadata_result.is_error is False
    metadata_text = result_text(metadata_result)
    assert MCP_TEST_SCHEMA in metadata_text
    assert MCP_TEST_VIEW in metadata_text
    assert query_result.is_error is False
    query_text = result_text(query_result)
    assert "USER_NAME" in query_text
    assert MCP_TEST_VALUE in query_text
    assert row_limit_result.is_error is False
    row_limit_payload = json.loads(result_text(row_limit_result))
    assert len(row_limit_payload["rows"]) == MCP_ROW_LIMIT


def assert_mcp_unreachable(endpoint: str) -> None:
    with requests.Session() as session:
        session.trust_env = False
        try:
            response = session.get(urljoin(endpoint, "/health"), timeout=2)
        except requests.RequestException:
            return
    pytest.fail(
        f"MCP sidecar remained reachable after disable: HTTP {response.status_code}"
    )


def test_mcp_sidecar_complete_local_lifecycle(
    mcp_deployment: SidecarDeployment,
) -> None:
    # Given
    host_port = available_loopback_port()
    mcp_deployment.sidecar("enable")
    set_host_port(mcp_deployment, host_port)
    # When
    mcp_deployment.deployment.deploy()
    mcp_deployment.sql(MCP_TEST_METADATA_SQL)
    initial_status = mcp_deployment.sidecar("status")
    # Then
    initial_endpoint = running_endpoint(initial_status, host_port)
    assert_mcp_interaction(initial_endpoint)

    # When
    mcp_deployment.deployment.stop()
    stopped_status = mcp_deployment.sidecar("status")
    # Then
    assert stopped_status["enabled"] is True
    assert stopped_status["running"] is False
    assert stopped_status["endpoints"] == []

    # When
    mcp_deployment.deployment.start()
    restarted_status = mcp_deployment.sidecar("status")
    # Then
    restarted_endpoint = running_endpoint(restarted_status, host_port)
    assert_mcp_interaction(restarted_endpoint)

    # When
    disabled_status = mcp_deployment.sidecar("disable")
    assert_mcp_unreachable(restarted_endpoint)
    preserved_data = mcp_deployment.sql(
        "SELECT USER_NAME FROM MCP_SIDECAR_TEST.ACTIVE_USER;"
    )
    # Then
    assert disabled_status["enabled"] is False
    assert disabled_status["running"] is False
    assert disabled_status["reconciliation"] == "complete"
    assert disabled_status["endpoints"] == []
    assert MCP_TEST_VALUE in preserved_data

    # When
    reenabled_status = mcp_deployment.sidecar("enable")
    # Then
    assert reenabled_status["health"] == "serving"
    assert_mcp_interaction(running_endpoint(reenabled_status, host_port))


def test_mcp_sidecar_rejects_rebound_requests(
    mcp_deployment: SidecarDeployment,
) -> None:
    # Given
    host_port = available_loopback_port()
    mcp_deployment.sidecar("enable")
    set_host_port(mcp_deployment, host_port)
    mcp_deployment.deployment.deploy()
    endpoint = running_endpoint(mcp_deployment.sidecar("status"), host_port)
    body = {
        "jsonrpc": "2.0",
        "id": 1,
        "method": "initialize",
        "params": {
            "protocolVersion": "2025-06-18",
            "capabilities": {},
            "clientInfo": {"name": "rebind-probe", "version": "0"},
        },
    }
    headers = {
        "Content-Type": "application/json",
        "Accept": "application/json, text/event-stream",
    }
    # When
    with requests.Session() as session:
        accepted = session.post(endpoint, json=body, headers=headers, timeout=30)
        foreign_host = session.post(
            endpoint, json=body, headers={**headers, "Host": "evil.example"}, timeout=30
        )
        foreign_origin = session.post(
            endpoint,
            json=body,
            headers={**headers, "Origin": "http://evil.example"},
            timeout=30,
        )
    # Then
    assert accepted.status_code == HTTPStatus.OK, accepted.text
    assert foreign_host.status_code >= HTTPStatus.BAD_REQUEST, foreign_host.status_code
    assert foreign_origin.status_code >= HTTPStatus.BAD_REQUEST, (
        foreign_origin.status_code
    )
