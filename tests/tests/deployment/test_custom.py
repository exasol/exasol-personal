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


@pytest.fixture
def instance_type(request: pytest.FixtureRequest) -> str | None:
    # Pass the parametrized value into the custom_deployment fixture
    return getattr(request, "param", None)


@pytest.fixture
def custom_deployment(
    exasol_path: str,
    instance_type: str | None,
) -> Iterator[tuple[Deployment, DeploymentConfig]]:
    """Deployment with custom parameters to test all customization options."""
    test_pw: Final = """$x\n${y}\nunbalanced quote: ' another one: " $(echo test)"""
    test_admin_pw: Final = "MyAdminUI!Pass123"

    if not instance_type:
        instance_type = "t3.xlarge"

    config = DeploymentConfig(
        infra="aws",
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


@pytest.mark.usefixtures("instance_type")
@pytest.mark.parametrize("instance_type", ["t3.micro", "t2.small"], indirect=True)
def test_custom_deployment_rejects_small_instance_types(
    request: pytest.FixtureRequest,
) -> None:
    """Deployment should fail for undersized instance types."""
    try:
        _deployment, config = request.getfixturevalue("custom_deployment")
        assert config.instance_type in ("t3.micro", "t2.small")
        pytest.fail(f"Deployment should fail for instance_type={config.instance_type}")
    except (subprocess.CalledProcessError, RuntimeError):
        # Expected failure from Terraform or deployment layer - pass
        inst_type = request.getfixturevalue("instance_type")
        logging.info(
            "Expected deployment failure for instance_type=%s",
            inst_type,
        )
        return
    except Exception as unexpected:
        inst_type = request.getfixturevalue("instance_type")
        logging.exception(
            "Unexpected exception occurred for instance_type=%s",
            inst_type,
        )
        pytest.fail(f"Unexpected exception type: {type(unexpected).__name__}")


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
def test_custom_deployment_success(
    custom_deployment: tuple[Deployment, DeploymentConfig],
) -> None:
    deployment, config = custom_deployment
    query: Final = "SELECT * FROM Dual"

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
