# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

"""Deploy ops: idempotency, info/outputs, destroy, connection-info normalization."""

import logging
import subprocess
import sys
from collections.abc import Iterator

import pytest

from framework.deployment import Deployment, StatusDatabaseReady, StatusInitialized
from framework.launcher import DeploymentConfig, Launcher
from framework.outputs import get_outputs
from tests.testcase_helpers import skip_unless_infra


@pytest.fixture(scope="module")
def idempotency_deployment(exasol_path: str, infra: str) -> Iterator[Deployment]:
    """Deploy once and tear down after the module's tests finish."""
    cluster_size = 2 if infra == "aws" else 1
    config = DeploymentConfig(infra=infra, cluster_size=cluster_size)
    deployment = Deployment(Launcher(exasol_path), config=config)
    try:
        deployment.deploy()
        if not deployment.has_status(StatusDatabaseReady):
            msg = f"Expected status `{StatusDatabaseReady}` after initial deploy"
            raise RuntimeError(msg)
        yield deployment
    finally:
        deployment.cleanup()


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
def test_deploy_is_idempotent(idempotency_deployment: Deployment) -> None:
    """Re-running deploy on a healthy cluster must not corrupt it.

    Either the second deploy is a clean no-op (returncode 0) or it fails
    with a message that clearly signals the deployment already exists.
    Either way: deployment id, status, and DB connectivity stay intact.
    """
    # ========== GIVEN ==========
    # A successfully deployed, healthy cluster from the module fixture.
    assert idempotency_deployment.has_status(StatusDatabaseReady)
    assert idempotency_deployment.db_connectable()
    deployment_id_before = idempotency_deployment.deployment_id()

    # ========== WHEN ==========
    # We run deploy a second time against the same deployment dir / config.
    try:
        second_deploy = idempotency_deployment.deploy()
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
    assert idempotency_deployment.has_status(StatusDatabaseReady)
    assert idempotency_deployment.deployment_id() == deployment_id_before
    assert idempotency_deployment.db_connectable()


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
def test_info_includes_connection_details(reusable_deployment: Deployment) -> None:
    """A deployed cluster must surface host, port, AdminUI URL.

    The launcher does not print SQL host/port to deploy's stdout today; they
    are exposed via `exasol info` (human-readable) and via the deployment
    outputs file (machine-readable). Both surfaces are asserted here.
    """
    # ========== GIVEN ==========
    # A successfully deployed cluster
    assert reusable_deployment.db_connectable()

    # ========== WHEN ==========
    # We query `info` and inspect the structured outputs
    info_result = reusable_deployment.info()
    outputs = get_outputs(reusable_deployment.deployment_dir.name)

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


@pytest.fixture
def destroyable_deployment(exasol_path: str, infra: str) -> Iterator[Deployment]:
    """Yield a short-lived deployment dedicated to exercising destroy."""
    cluster_size = 2 if infra == "aws" else 1
    config = DeploymentConfig(infra=infra, cluster_size=cluster_size)
    deployment = Deployment(Launcher(exasol_path), config=config)
    try:
        deployment.deploy()
        yield deployment
    finally:
        # Destroy may have already removed resources; cleanup() is idempotent.
        deployment.cleanup()


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
def test_destroy_removes_deployment(destroyable_deployment: Deployment) -> None:
    """Destroy must remove cloud resources and clear deployment state."""
    # ========== GIVEN ==========
    # A successfully deployed cluster
    assert destroyable_deployment.db_connectable()

    # ========== WHEN ==========
    # The user runs the documented destroy command
    result = destroyable_deployment.destroy("--auto-approve")

    # ========== THEN ==========
    # Destroy completes successfully and the deployment is gone
    assert result.returncode == 0
    assert destroyable_deployment.has_status(StatusInitialized)
    assert destroyable_deployment.has_no_deployment()


@pytest.mark.installation_e2e
def test_connection_info_normalization_and_legacy_fallback(infra: str) -> None:
    skip_unless_infra(infra, "aws", "azure", "exoscale", "stackit", "local")

    # Exercising both the new connection block and the legacy node-based
    # fallback requires a real deployment.json produced by a deployment.
    pytest.skip("requires a deployed deployment.json (new and legacy shapes)")
