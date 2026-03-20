# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

"""Tests using a deployment with custom configuration."""

import logging
import subprocess
import sys
import textwrap
from collections.abc import Iterator
from typing import Final

import pytest
import semver

from framework.deployment import Deployment
from framework.launcher import DeploymentConfig, Launcher

pytestmark = [pytest.mark.infrastructure_e2e]


@pytest.fixture
def instance_type(request: pytest.FixtureRequest) -> str | None:
    # Pass the parametrized value into the custom_deployment fixture
    return getattr(request, "param", None)


@pytest.fixture
def custom_deployment(
    exasol_path: str,
    infra: str,
    instance_type: str | None,
) -> Iterator[tuple[Deployment, DeploymentConfig]]:
    """Deployment with custom parameters to test all customization options."""
    test_pw: Final = """$x\n${y}\nunbalanced quote: ' another one: " $(echo test)"""
    test_admin_pw: Final = "MyAdminUI!Pass123"

    if instance_type is None:
        if infra == "aws":
            instance_type = "t3.xlarge"
        elif infra == "exoscale":
            # Use a non-default Exoscale instance type in custom deployment tests
            # to validate instance-type customization works.
            instance_type = "standard.large"

    config = DeploymentConfig(
        infra=infra,
        cluster_size=1,
        instance_type=instance_type,
        data_volume_size=120,
        db_password=test_pw,
        adminui_password=test_admin_pw,
    )

    deployment = Deployment(
        Launcher(exasol_path),
        config=config,
    )
    try:
        deployment.deploy()

        yield (deployment, config)

    finally:
        deployment.cleanup()


@pytest.mark.parametrize(
    ("target_infra", "instance_type"),
    [
        pytest.param(
            "aws",
            "t3.micro",
            marks=pytest.mark.provider_aws,
            id="aws-t3.micro",
        ),
        pytest.param(
            "aws",
            "t2.small",
            marks=pytest.mark.provider_aws,
            id="aws-t2.small",
        ),
        pytest.param(
            "azure",
            "Standard_B2s",
            marks=pytest.mark.provider_azure,
            id="azure-Standard_B2s",
        ),
    ],
    indirect=["instance_type"],
)
@pytest.mark.usefixtures("instance_type")
def test_custom_deployment_rejects_small_instance_types(
    request: pytest.FixtureRequest,
    infra: str,
    target_infra: str,
) -> None:
    """Deployment should fail for undersized instance types."""
    if infra != target_infra:
        pytest.skip(f"{target_infra}-specific instance type validation")

    expected_instance_type = request.getfixturevalue("instance_type")

    try:
        _deployment, config = request.getfixturevalue("custom_deployment")
        assert config.instance_type == expected_instance_type
        pytest.fail(f"Deployment should fail for instance_type={config.instance_type}")
    except (subprocess.CalledProcessError, RuntimeError):
        # Expected failure from Terraform or deployment layer - pass
        logging.info(
            "Expected deployment failure for instance_type=%s",
            expected_instance_type,
        )
        return
    except Exception as unexpected:
        logging.exception(
            "Unexpected exception occurred for instance_type=%s",
            expected_instance_type,
        )
        pytest.fail(f"Unexpected exception type: {type(unexpected).__name__}")


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
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
        stderr = proc.stderr.strip()
        stdout = proc.stdout.strip()

        lines = stderr.splitlines()
        version_line, exit_hint_lint = lines[0], lines[1]

        assert exit_hint_lint.strip() == 'Type "exit" to exit the shell'

        # Check the Exasol version that is printed as the first line
        exasol_name, exasol_version = version_line.split(" ")

        assert exasol_name == "Exasol"
        assert semver.VersionInfo.is_valid(exasol_version)

        # Check the query output.
        expected = textwrap.dedent("""
        ┌───────┐
        │ DUMMY │
        ├───────┤
        │ <nil> │
        └───────┘
        """)

        assert stdout.strip("\n") == expected.strip("\n")
