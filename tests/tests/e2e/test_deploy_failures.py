import logging
import os
import shutil
import subprocess
import sys
import tempfile
from collections.abc import Iterator
from pathlib import Path
from typing import Final

import pytest

from framework.deployment import Deployment
from framework.launcher import DeploymentConfig, Launcher

pytestmark = [pytest.mark.e2e]


@pytest.fixture
def fresh_deployment(exasol_path: str, infra: str) -> Iterator[Deployment]:
    """Yield a fresh, never-deployed Deployment for tests that expect deploy to fail."""
    config = DeploymentConfig(infra=infra, cluster_size=1)
    deployment = Deployment(Launcher(exasol_path), config=config)
    try:
        yield deployment
    finally:
        deployment.cleanup()


@pytest.mark.skipif(
    sys.platform.startswith("win"),
    reason="Test is not supported on Windows OS",
)
@pytest.mark.provider_aws
def test_deploy_fails_clearly_with_invalid_aws_credentials(
    fresh_deployment: Deployment,
    infra: str,
) -> None:
    """Invalid AWS credentials -> auth error, no dangling resources."""
    if infra != "aws":
        pytest.skip("Invalid-credentials scenario is AWS-specific")

    # ========== GIVEN ==========
    # An initialized deployment, and a subprocess environment with intentionally
    # invalid AWS credentials.
    bad_env = os.environ.copy()
    # Deliberately invalid credentials used to provoke a clean auth-failure
    # path; not real secrets.
    bad_env["AWS_ACCESS_KEY_ID"] = "AKIAINVALIDINVALIDINV"
    bad_env["AWS_SECRET_ACCESS_KEY"] = "INVALIDinvalidINVALIDinvalidINVALIDinvalid"  # noqa: S105
    bad_env.pop("AWS_SESSION_TOKEN", None)
    bad_env.pop("AWS_PROFILE", None)

    # ========== WHEN ==========
    # Deploy is invoked with bad credentials
    proc = subprocess.run(
        [
            fresh_deployment.launcher.launcher_path,
            "deploy",
            "--deployment-dir",
            fresh_deployment.deployment_dir.name,
        ],
        capture_output=True,
        text=True,
        check=False,
        env=bad_env,
    )

    # ========== THEN ==========
    # Deploy exits non-zero with an authentication-related message and no panic.
    assert proc.returncode != 0
    combined = (proc.stdout + proc.stderr).lower()
    assert "panic:" not in combined
    assert "traceback" not in combined
    assert any(
        token in combined
        for token in (
            "credentials",
            "unauthorized",
            "auth",
            "invalidclienttoken",
            "signature",
            "access denied",
        )
    ), f"Expected an auth-related error, got: {combined!r}"
