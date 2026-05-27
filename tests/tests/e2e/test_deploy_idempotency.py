import logging
import subprocess
import sys
from collections.abc import Iterator

import pytest

from framework.deployment import Deployment, StatusDatabaseReady
from framework.launcher import DeploymentConfig, Launcher

pytestmark = [pytest.mark.e2e]


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
