# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

"""Deploy ops: idempotency, info/outputs, destroy, connection-info normalization."""

import logging
import subprocess
import sys

import pytest

from framework.deployment import Deployment, StatusDatabaseReady, StatusInitialized
from framework.outputs import get_outputs


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
@pytest.mark.openspec("deployment-lifecycle-recovery")
def test_deploy_is_idempotent(reusable_live_deployment: Deployment) -> None:
    """Re-running deploy on a healthy cluster must not corrupt it.

    Either the second deploy is a clean no-op (returncode 0) or it fails
    with a message that clearly signals the deployment already exists.
    Either way: deployment id, status, and DB connectivity stay intact.
    """
    # ========== GIVEN ==========
    # A successfully deployed, healthy cluster from the module fixture.
    assert reusable_live_deployment.has_status(StatusDatabaseReady)
    assert reusable_live_deployment.db_connectable()
    deployment_id_before = reusable_live_deployment.deployment_id()

    # ========== WHEN ==========
    # We run deploy a second time against the same deployment dir / config.
    try:
        second_deploy = reusable_live_deployment.deploy()
        second_returncode = second_deploy.returncode
        second_stdout = second_deploy.stdout or ""
        second_stderr = second_deploy.stderr or ""
    except subprocess.CalledProcessError as exc:
        # A non-zero exit is acceptable per the spec, but the message must
        # make it clear the deployment already exists.
        second_returncode = exc.returncode
        second_stdout = exc.stdout or ""
        second_stderr = exc.stderr or ""
        combined = f"{second_stdout}\n{second_stderr}".lower()
        idempotency_phrases = (
            "already",
            "exists",
            "no changes",
            "up-to-date",
            "up to date",
        )
        assert any(phrase in combined for phrase in idempotency_phrases), (
            f"Second deploy failed with returncode {second_returncode} but the "
            "output does not explain that the deployment already exists.\n"
            f"stdout: {second_stdout}\nstderr: {second_stderr}"
        )

    logging.info(
        "Second deploy returncode=%s stdout_len=%d stderr_len=%d",
        second_returncode,
        len(second_stdout),
        len(second_stderr),
    )

    # ========== THEN ==========
    # The deployment must remain intact: same id, ready status, DB reachable.
    assert reusable_live_deployment.has_status(StatusDatabaseReady)
    assert reusable_live_deployment.deployment_id() == deployment_id_before
    assert reusable_live_deployment.db_connectable()


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
@pytest.mark.openspec("deployment-info-reporting")
@pytest.mark.providers("aws", "azure", "exoscale")
def test_info_includes_connection_details(
    shared_live_deployment: Deployment,
) -> None:
    """A deployed cluster must surface host, port, AdminUI URL.

    The launcher does not print SQL host/port to deploy's stdout today; they
    are exposed via `exasol info` (human-readable) and via the deployment
    outputs file (machine-readable). Both surfaces are asserted here.
    """
    # ========== GIVEN ==========
    # A successfully deployed cluster
    assert shared_live_deployment.db_connectable()

    # ========== WHEN ==========
    # We query `info` and inspect the structured outputs
    info_result = shared_live_deployment.info()
    outputs = get_outputs(shared_live_deployment.deployment_dir.name)

    # ========== THEN ==========
    # Info exits successfully and exposes the cluster details a user needs
    assert info_result.returncode == 0
    stdout = info_result.stdout
    assert "Exasol Personal" in stdout
    assert "Cluster Size:" in stdout
    assert "Cluster State: running" in stdout

    # And the outputs file exposes a full connection record for at least one node
    assert outputs.deploymentId
    assert outputs.nodes, "Outputs file should list at least one node"
    node = next(iter(outputs.nodes.values()))
    assert node.publicIp
    assert node.database.dbPort
    assert node.database.uiPort
    assert node.database.url
    assert node.ssh.command
    assert node.ssh.username


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
@pytest.mark.openspec("deployment-lifecycle-recovery")
def test_destroy_removes_deployment(isolated_live_deployment: Deployment) -> None:
    """Destroy must remove cloud resources and clear deployment state."""
    # ========== GIVEN ==========
    # A successfully deployed cluster
    assert isolated_live_deployment.db_connectable()

    # ========== WHEN ==========
    # The user runs the documented destroy command
    result = isolated_live_deployment.destroy("--auto-approve")

    # ========== THEN ==========
    # Destroy completes successfully and the deployment is gone
    assert result.returncode == 0
    assert isolated_live_deployment.has_status(StatusInitialized)
    assert isolated_live_deployment.has_no_deployment()
