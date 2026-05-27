import sys
from collections.abc import Iterator

import pytest

from framework.deployment import Deployment, StatusInitialized
from framework.launcher import DeploymentConfig, Launcher

pytestmark = [pytest.mark.e2e]


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
