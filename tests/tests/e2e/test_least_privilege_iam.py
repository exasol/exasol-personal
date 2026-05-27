import os
import subprocess
import sys
from collections.abc import Iterator

import pytest

from framework.deployment import Deployment
from framework.launcher import DeploymentConfig, Launcher

pytestmark = [pytest.mark.e2e, pytest.mark.provider_aws]

_PROFILE_ENV: str = "EXASOL_TEST_LEAST_PRIV_PROFILE"


@pytest.fixture
def least_priv_deployment(exasol_path: str) -> Iterator[Deployment]:
    config = DeploymentConfig(infra="aws", cluster_size=1)
    deployment = Deployment(Launcher(exasol_path), config=config)
    try:
        yield deployment
    finally:
        deployment.cleanup()


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
@pytest.mark.skipif(
    os.environ.get(_PROFILE_ENV, "") == "",
    reason=(
        f"{_PROFILE_ENV} is not set; provision a least-privilege AWS profile and "
        "point this env var at it to enable PDF #7."
    ),
)
def test_deploy_with_least_privilege_iam_names_missing_action(
    least_priv_deployment: Deployment,
) -> None:
    """Deploy under a least-privilege role must name the missing action."""
    # ========== GIVEN ==========
    # An initialized deployment and an AWS profile missing some required action
    env = os.environ.copy()
    env["AWS_PROFILE"] = env[_PROFILE_ENV]

    # ========== WHEN ==========
    proc = subprocess.run(
        [
            least_priv_deployment.launcher.launcher_path,
            "deploy",
            "--deployment-dir",
            least_priv_deployment.deployment_dir.name,
        ],
        capture_output=True,
        text=True,
        check=False,
        env=env,
    )

    # ========== THEN ==========
    # Deploy fails with an authorization error that names an AWS action
    assert proc.returncode != 0
    combined = (proc.stdout + proc.stderr).lower()
    assert "panic:" not in combined
    assert (
        "unauthorized" in combined
        or "not authorized" in combined
        or ("accessdenied" in combined.replace(" ", ""))
    )
    # An IAM-aware message references a specific ec2:/iam: action
    assert "ec2:" in combined or "iam:" in combined or "s3:" in combined, (
        f"Error did not name a missing AWS action: {combined!r}"
    )
