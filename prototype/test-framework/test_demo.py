# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT
# ruff: noqa: INP001

from __future__ import annotations

from typing import TYPE_CHECKING

import pytest

if TYPE_CHECKING:
    from conftest import FakeDeployment


@pytest.mark.openspec("runtime-artifact-cache")
def test_controlled_launcher_behavior(deployment: FakeDeployment) -> None:
    # Given
    assert deployment.boundary == "controlled"

    # When
    result = deployment.query("SELECT 1")

    # Then
    assert result == 1


@pytest.mark.openspec("deployment-info-reporting")
@pytest.mark.smoke
def test_shared_deployment_is_ready(
    shared_live_deployment: FakeDeployment,
) -> None:
    # Given
    assert shared_live_deployment.boundary == "shared-live"

    # When
    result = shared_live_deployment.query("SELECT 1")

    # Then
    assert result == 1


@pytest.mark.openspec("connect-sql-input")
def test_shared_deployment_is_reused(
    shared_live_deployment: FakeDeployment,
) -> None:
    # Given
    assert shared_live_deployment.state == "ready"

    # When
    result = shared_live_deployment.query("SELECT 2")

    # Then
    assert result == 1


@pytest.mark.openspec("admin-ui-access")
@pytest.mark.providers("aws", "azure")
def test_provider_specific_capability(
    shared_live_deployment: FakeDeployment,
) -> None:
    # Given
    assert shared_live_deployment.provider in {"aws", "azure"}

    # When
    result = shared_live_deployment.query("SELECT CURRENT_USER")

    # Then
    assert result == 1


@pytest.mark.openspec("deployment-lifecycle-recovery")
@pytest.mark.chaos
@pytest.mark.parametrize("failure", ["network", "process"])
def test_isolated_lifecycle_recovery(
    isolated_live_deployment: FakeDeployment,
    failure: str,
) -> None:
    # Given
    assert isolated_live_deployment.state == "ready"
    assert failure in {"network", "process"}

    # When
    isolated_live_deployment.stop()
    isolated_live_deployment.start()

    # Then
    assert isolated_live_deployment.state == "ready"


@pytest.mark.openspec("exasol-local-deployment")
@pytest.mark.providers("local")
@pytest.mark.platform("linux-amd64", "macos-arm64")
@pytest.mark.smoke
def test_local_runtime(local_deployment: FakeDeployment) -> None:
    # Given
    assert local_deployment.boundary == "local"

    # When
    result = local_deployment.query("SELECT 1")

    # Then
    assert result == 1
