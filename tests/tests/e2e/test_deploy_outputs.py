import sys

import pytest

from framework.deployment import Deployment
from framework.outputs import get_outputs

pytestmark = [pytest.mark.e2e]


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
def test_info_includes_connection_details(e2e_deployment: Deployment) -> None:
    """A deployed cluster must surface host, port, AdminUI URL.

    The launcher does not print SQL host/port to deploy's stdout today; they
    are exposed via `exasol info` (human-readable) and via the deployment
    outputs file (machine-readable). Both surfaces are asserted here.
    """
    # ========== GIVEN ==========
    # A successfully deployed cluster
    assert e2e_deployment.db_connectable()

    # ========== WHEN ==========
    # We query `info` and inspect the structured outputs
    info_result = e2e_deployment.info()
    outputs = get_outputs(e2e_deployment.deployment_dir.name)

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
