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


@pytest.mark.skipif(
    sys.platform.startswith("win"),
    reason="Test is not supported on Windows OS",
)
def test_deploy_fails_with_invalid_terraform_template(
    exasol_path: str,
    infra: str,
) -> None:
    """A user-corrupted TF template must produce an actionable failure.

    We export the infra preset to a writable directory, inject a syntax error,
    and use the corrupted preset path with `init`. The subsequent deploy must
    fail with a Terraform validation/parse error rather than a panic.
    """
    # ========== GIVEN ==========
    # An exported infra preset whose main.tf has been corrupted with invalid HCL
    work_dir = Path(tempfile.mkdtemp(prefix="exasol-tf-corrupt-"))
    preset_dir = work_dir / "infra-preset"
    deployment_dir = work_dir / "deployment"
    deployment_dir.mkdir()

    try:
        subprocess.run(
            [
                exasol_path,
                "presets",
                "export",
                infra,
                "--type",
                "infrastructures",
                "--to",
                str(preset_dir),
            ],
            check=True,
            capture_output=True,
            text=True,
        )

        main_tf = preset_dir / "main.tf"
        if not main_tf.exists():
            tf_files = sorted(preset_dir.rglob("*.tf"))
            assert tf_files, f"Exported preset has no .tf files under {preset_dir}"
            main_tf = tf_files[0]

        original = main_tf.read_text(encoding="utf-8")
        # Inject obviously invalid HCL at the end of the file
        main_tf.write_text(
            original + '\n\nresource "invalid syntax {{ this is not valid HCL\n',
            encoding="utf-8",
        )

        # Init with the corrupted preset path
        subprocess.run(
            [
                exasol_path,
                "init",
                str(preset_dir),
                "--deployment-dir",
                str(deployment_dir),
            ],
            check=True,
            capture_output=True,
            text=True,
        )

        # ========== WHEN ==========
        # Deploy is invoked against the corrupted template
        proc = subprocess.run(
            [
                exasol_path,
                "deploy",
                "--deployment-dir",
                str(deployment_dir),
            ],
            capture_output=True,
            text=True,
            check=False,
        )

        # ========== THEN ==========
        # Deploy fails fast with a parse/validation error and no panic
        assert proc.returncode != 0
        combined = (proc.stdout + proc.stderr).lower()
        assert "panic:" not in combined
        assert "traceback" not in combined
        assert any(
            token in combined
            for token in (
                "syntax",
                "parse",
                "invalid",
                "error",
                "expected",
                ".tf",
            )
        ), f"Expected a template error, got: {combined!r}"

        # And no .tfstate with resources was written
        terraform_dir: Final = deployment_dir / ".terraform"
        for state_path in deployment_dir.rglob("terraform.tfstate"):
            content = state_path.read_text(encoding="utf-8", errors="replace")
            # A failed plan should not have populated resources
            assert '"resources": [' not in content or '"resources": []' in content, (
                f"Resources unexpectedly recorded in state at {state_path}"
            )
        logging.info(
            "Terraform plan failed as expected; .terraform dir present=%s",
            terraform_dir.exists(),
        )
    finally:
        # Best-effort cleanup of the work directory (no cloud resources to free)
        shutil.rmtree(work_dir, ignore_errors=True)
