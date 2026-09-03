# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

"""Object-storage bootstrap, IAM, privacy, versions, and runtime cache (cloud)."""

import json
from pathlib import Path

import pytest

from tests.testcase_helpers import (
    run_command,
    skip_unless_infra,
    skip_without_cloud_deploy_optin,
)

REPO_ROOT = Path(__file__).resolve().parents[3]


@pytest.mark.provider_aws
@pytest.mark.infrastructure_e2e
def test_minimal_iam_policy_includes_bootstrap_bucket_actions(infra: str) -> None:
    skip_unless_infra(infra, "aws")

    # The minimal policy document must include the object-storage bootstrap
    # actions so that a role limited to it can still deploy. The full deploy
    # under that role is a cloud test; here we assert the policy contents that
    # make it possible.
    policy_path = (
        REPO_ROOT / "assets" / "infrastructure" / "aws" / "iam-policy.minimal.json"
    )
    if not policy_path.exists():
        pytest.skip(f"minimal IAM policy not found at {policy_path}")

    document = json.dumps(json.loads(policy_path.read_text()))
    assert "s3:PutObject" in document
    assert (
        "s3:PutBucketPolicy" in document or "s3:PutBucketPublicAccessBlock" in document
    )


@pytest.mark.infrastructure_e2e
def test_first_run_downloads_then_reuses_opentofu(
    exasol_path: str, infra: str, tmp_path: Path
) -> None:
    # Given a cloud infrastructure preset (OpenTofu-backed) is selected
    skip_unless_infra(infra, "aws", "azure", "exoscale", "stackit")
    skip_without_cloud_deploy_optin()

    # Given
    run_command([exasol_path, "cache", "clean", "--all"])

    # When a deployment that resolves OpenTofu runs (install performs init+deploy)
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()
    base = ["--deployment-dir", str(deployment_dir), "--no-launcher-version-check"]
    try:
        run_command([exasol_path, "install", infra, *base])

        # Then the cache now contains a resolved artifact
        listing = run_command([exasol_path, "cache", "list"]).stdout
        assert "No cached resources." not in listing

        # And a second resolve reuses the cache (no error, artifact still present)
        run_command([exasol_path, "deploy", *base])
        assert "No cached resources." not in (
            run_command([exasol_path, "cache", "list"]).stdout
        )
    finally:
        run_command([exasol_path, "destroy", "--remove", "--auto-approve", *base])
