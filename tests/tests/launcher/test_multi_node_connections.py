# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

"""Multi-node support is retained in each initialized cloud installation preset."""

from pathlib import Path

import pytest
import yaml

from .helpers import run_command

pytestmark = pytest.mark.openspec("multi-node-connections")


def test_cloud_init_records_multi_node_support(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given
    deployment = tmp_path / "cloud"

    # When
    run_command(
        [
            exasol_path,
            "init",
            "aws",
            "--deployment-dir",
            str(deployment),
            "--no-launcher-version-check",
        ]
    )

    # Then
    manifest = yaml.safe_load(
        (deployment / "installation/installation.yaml").read_text()
    )
    assert manifest["databaseConnection"]["allowAllNodes"] is True
