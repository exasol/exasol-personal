# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

import json
from pathlib import Path

import pytest

pytestmark = pytest.mark.openspec("bootstrap-asset-distribution")

REPO_ROOT = Path(__file__).resolve().parents[3]


def test_minimal_iam_policy_includes_bootstrap_bucket_actions() -> None:
    # Given
    policy_path = (
        REPO_ROOT / "assets" / "infrastructure" / "aws" / "iam-policy.minimal.json"
    )
    if not policy_path.exists():
        pytest.skip(f"minimal IAM policy not found at {policy_path}")

    # When
    document = json.dumps(json.loads(policy_path.read_text()))

    # Then
    assert "s3:PutObject" in document
    assert (
        "s3:PutBucketPolicy" in document or "s3:PutBucketPublicAccessBlock" in document
    )
