# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

"""Runtime artifact cache behavior against a live deployment."""

import pytest

from framework.deployment import Deployment
from tests.testcase_helpers import run_command


@pytest.mark.openspec("runtime-artifact-cache")
@pytest.mark.providers("aws", "azure", "exoscale")
def test_first_run_downloads_then_reuses_opentofu(
    reusable_live_deployment: Deployment,
) -> None:
    launcher_path = reusable_live_deployment.launcher.launcher_path

    listing = run_command([launcher_path, "cache", "list"]).stdout
    assert "No cached runtime artifacts." not in listing

    reusable_live_deployment.deploy()
    assert "No cached runtime artifacts." not in (
        run_command([launcher_path, "cache", "list"]).stdout
    )
