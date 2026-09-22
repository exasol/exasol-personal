# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

"""Tests using a deployment with custom configuration."""

import subprocess
import textwrap
from collections.abc import Iterator
from typing import Final

import pytest

from framework.deployment import Deployment, DeploymentManager
from framework.launcher import DeploymentConfig

pytestmark = pytest.mark.openspec("deployment-reconfiguration")

TEST_DB_PASSWORD: Final = "$x\n${y}\nunbalanced quote: ' another one: \" $(echo test)"  # noqa: S105
TEST_ADMINUI_PASSWORD: Final = "MyAdminUI!Pass123"  # noqa: S105


@pytest.fixture
def instance_type(request: pytest.FixtureRequest) -> str | None:
    # Pass the parametrized value into the custom_deployment fixture
    return getattr(request, "param", None)


def _custom_config(infra: str, instance_type: str | None) -> DeploymentConfig:
    if instance_type is None:
        instance_type = {
            "aws": "t3.xlarge",
            "exoscale": "standard.large",
        }.get(infra)

    return DeploymentConfig(
        infra=infra,
        cluster_size=1,
        instance_type=instance_type,
        data_volume_size=120,
        db_password=TEST_DB_PASSWORD,
        adminui_password=TEST_ADMINUI_PASSWORD,
    )


def _deploy_and_release(
    deployment_manager: DeploymentManager, config: DeploymentConfig
) -> None:
    deployment = deployment_manager.isolated_live(config=config)
    deployment_manager.release(deployment)


@pytest.fixture
def custom_deployment(
    deployment_manager: DeploymentManager,
    infra: str,
    instance_type: str | None,
) -> Iterator[tuple[Deployment, DeploymentConfig]]:
    """Deployment with custom parameters to test all customization options."""
    config = _custom_config(infra, instance_type)

    deployment = deployment_manager.isolated_live(config=config)
    try:
        yield (deployment, config)

    finally:
        deployment_manager.release(deployment)


@pytest.mark.parametrize(
    ("target_infra", "instance_type"),
    [
        pytest.param(
            "aws",
            "t3.micro",
            marks=pytest.mark.providers("aws"),
            id="aws-t3.micro",
        ),
        pytest.param(
            "aws",
            "t2.small",
            marks=pytest.mark.providers("aws"),
            id="aws-t2.small",
        ),
        pytest.param(
            "azure",
            "Standard_B2s",
            marks=pytest.mark.providers("azure"),
            id="azure-Standard_B2s",
        ),
    ],
    indirect=["instance_type"],
)
def test_custom_deployment_rejects_small_instance_types(
    deployment_manager: DeploymentManager,
    infra: str,
    instance_type: str | None,
    target_infra: str,
) -> None:
    """Deployment should fail for undersized instance types."""
    if infra != target_infra:
        pytest.skip(f"{target_infra}-specific instance type validation")

    config = _custom_config(infra, instance_type)

    # Deployment is expected to fail (via Terraform or the deployment layer) for
    # undersized instance types. Any other exception fails the test with its own
    # traceback; if deployment succeeds instead, pytest.raises fails the test.
    with pytest.raises((subprocess.CalledProcessError, RuntimeError)):
        _deploy_and_release(deployment_manager, config)


@pytest.mark.providers("aws", "azure", "exoscale")
def test_custom_deployment_success(
    custom_deployment: tuple[Deployment, DeploymentConfig],
    infra: str,
) -> None:
    deployment, config = custom_deployment
    query: Final = "SELECT * FROM Dual"

    if infra == "exoscale":
        assert config.instance_type == "standard.large"

    assert config.db_password is not None
    for p in [(), ("--password", config.db_password)]:
        proc = deployment.connect(*p, input=query, capture_output=True)
        stdout = proc.stdout.strip()

        # Check the query output.
        expected = textwrap.dedent("""
        ┌───────┐
        │ DUMMY │
        ├───────┤
        │ <nil> │
        └───────┘
        """)

        assert stdout.strip("\n") == expected.strip("\n")
