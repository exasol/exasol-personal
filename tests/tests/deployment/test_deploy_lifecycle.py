# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

"""Lifecycle and provisioning tests against the shared reusable deployment.

The ``reusable_deployment`` fixture (session-scoped, defined in the root conftest)
is shared with the e2e and chaos suites. Tests here mutate lifecycle state but must
leave the deployment database-ready on exit.
"""

import sys

import pytest
import requests

from framework.deployment import Deployment, StatusDatabaseReady


@pytest.mark.infrastructure_e2e
def test_stop_and_start(reusable_deployment: Deployment) -> None:
    # Using resuable_deployment fixture
    assert reusable_deployment.db_connectable()

    # Test info before stopping - should show running state
    info_result = reusable_deployment.info()
    assert info_result.returncode == 0
    assert "Exasol Personal" in info_result.stdout
    assert "Cluster Size:" in info_result.stdout
    assert "Cluster State: running" in info_result.stdout

    # Stop the deployment
    stop_result = reusable_deployment.stop()
    assert hasattr(stop_result, "returncode")
    assert stop_result.returncode == 0

    # Test info after stopping - should show stopped state
    info_result = reusable_deployment.info()
    assert info_result.returncode == 0
    assert "Exasol Personal" in info_result.stdout
    assert "Cluster Size:" in info_result.stdout
    assert "Cluster State: stopped" in info_result.stdout

    # Start the deployment again
    start_result = reusable_deployment.start()
    assert hasattr(start_result, "returncode")
    assert start_result.returncode == 0

    # Test info after starting - should show running state again
    info_result = reusable_deployment.info()
    assert info_result.returncode == 0
    assert "Exasol Personal" in info_result.stdout
    assert "Cluster Size:" in info_result.stdout
    assert "Cluster State: running" in info_result.stdout

    # Immediately verify DB is connectable after start completes
    assert reusable_deployment.db_connectable()

    # The interactive `connect()` spawns a shell that reads from stdin. On Windows
    # that path depends on the piped-stdin fix tracked separately (SPOT-31454), so
    # here we only assert the non-interactive stop/start/info lifecycle. Exercise
    # the interactive check on POSIX, where it is supported today.
    if not sys.platform.startswith("win"):
        connect_result = reusable_deployment.connect()
        assert hasattr(connect_result, "returncode")
        assert connect_result.returncode == 0


@pytest.mark.infrastructure_e2e
@pytest.mark.provider_aws
@pytest.mark.provider_azure
@pytest.mark.provider_stackit
def test_remote_archive_registered(
    reusable_deployment: Deployment,
    infra: str,
) -> None:
    """Scenario: Verify that a remote archive volume is registered via Admin UI.

    Given a deployed Exasol cluster with an initialized remote archive volume
    When we authenticate to the Admin UI API using the admin credentials
    And we list the available deployments
    And we query the backup options for the deployment
    Then we should receive a successful response
    And the response should contain the 'default_archive' backup option
    """
    if infra not in {"aws", "azure", "stackit"}:
        pytest.skip("Remote archive verification is only supported for AWS/Azure today")

    # ========== GIVEN ==========
    # Ensure deployment is running (previous tests may have stopped it)
    if not reusable_deployment.has_status(StatusDatabaseReady):
        start_result = reusable_deployment.start()
        assert start_result.returncode == 0
        assert reusable_deployment.has_status(StatusDatabaseReady)

    # A deployed Exasol cluster with remote archive configured
    host, port = reusable_deployment.admin_ui()
    username, password = reusable_deployment.admin_ui_credentials()
    deployment_id = reusable_deployment.deployment_id()

    base_url = f"https://{host}:{port}/api/v1"
    verify_ssl = False  # equivalent to curl -k (insecure)

    # ========== WHEN ==========
    # We request an access token from the Admin UI API
    token_url = f"{base_url}/token"
    token_payload = {
        "grant_type": "password",
        "username": username,
        "password": password,
    }

    token_response = requests.post(
        token_url,
        data=token_payload,
        headers={"Content-Type": "application/x-www-form-urlencoded"},
        verify=verify_ssl,
        timeout=30,
    )

    token_response.raise_for_status()
    access_token = token_response.json().get("access_token")

    # When we list the available deployments
    deployments_url = f"{base_url}/deployments"

    deployments_response = requests.get(
        deployments_url,
        headers={"Authorization": f"Bearer {access_token}"},
        verify=verify_ssl,
        timeout=30,
    )

    deployments_response.raise_for_status()

    # ========== THEN ==========
    # We should receive a list of deployments
    assert len(deployments_response.json()) > 0, "No deployments found"

    # When we query the backup options for the deployment
    backups_url = f"{base_url}/deployments/{deployment_id}/backups"

    backups_response = requests.options(
        backups_url,
        headers={
            "Accept": "application/json",
            "Authorization": f"Bearer {access_token}",
        },
        verify=verify_ssl,
        timeout=30,
    )

    backups_response.raise_for_status()

    # Then we should receive backup options
    assert len(backups_response.json()) > 0, "No backup options found"

    # And the response should contain the 'default_archive' option
    ok = any(
        backup.get("name") == "default_archive" for backup in backups_response.json()
    )
    assert ok, "Missing default_archive"
