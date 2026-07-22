# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

"""End-to-end tests for official and custom script language container installation.

SLC install is driven through the same CLI on every backend, so these tests are written
infra-aware: today they skip on non-local deployments (SLC is local-only) and extend to
cloud by relaxing that skip once the cloud backend supports SLC delivery.

The custom container is supplied by the runner via EXASOL_TEST_CUSTOM_SLC_FILE or
EXASOL_TEST_CUSTOM_SLC_URL; the custom test skips if neither is set, so no large
container is hard-coded into the suite.
"""

import os
import sys
import textwrap
from collections.abc import Iterator
from subprocess import CompletedProcess
from typing import Final

import pytest

from framework.deployment import Deployment
from framework.launcher import DeploymentConfig, Launcher

CUSTOM_SLC_FILE_ENV: Final = "EXASOL_TEST_CUSTOM_SLC_FILE"
CUSTOM_SLC_URL_ENV: Final = "EXASOL_TEST_CUSTOM_SLC_URL"
CUSTOM_ALIAS: Final = "MYPY3"


@pytest.fixture(scope="module")
def slc_deployment(exasol_path: str, infra: str) -> Iterator[Deployment]:
    if infra != "local":
        pytest.skip("SLC is currently supported only on local deployments")

    deployment = Deployment(Launcher(exasol_path), config=DeploymentConfig(infra=infra))
    try:
        deployment.deploy()
        yield deployment
    finally:
        deployment.cleanup()


def _slc(
    deployment: Deployment, *args: str, capture: bool = False
) -> CompletedProcess[str]:
    return deployment.launcher.run_command(
        "slc",
        deployment.deployment_dir.name,
        *args,
        capture_output=capture,
    )


def _run_scalar_udf(deployment: Deployment, alias: str, schema: str) -> str:
    """Create and run a trivial scalar UDF; return the connect stdout."""
    script = textwrap.dedent(
        f"""\
        DROP SCHEMA IF EXISTS {schema} CASCADE;
        CREATE SCHEMA {schema};
        OPEN SCHEMA {schema};
        CREATE OR REPLACE {alias} SCALAR SCRIPT hello() RETURNS VARCHAR(10) AS
        def run(ctx):
            return 'hi'
        /
        SELECT hello();
        """
    )
    return deployment.connect(input=script, capture_output=True).stdout


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
@pytest.mark.installation_e2e
@pytest.mark.local_e2e
def test_official_slc_install_runs_udf(slc_deployment: Deployment) -> None:
    """Official install makes PYTHON3 UDFs runnable."""
    _slc(slc_deployment, "install", "python3", "--auto-approve")

    assert "PYTHON3" in _slc(slc_deployment, "list", capture=True).stdout
    assert "hi" in _run_scalar_udf(slc_deployment, "PYTHON3", "slc_e2e_official")


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
@pytest.mark.installation_e2e
@pytest.mark.local_e2e
def test_custom_slc_install_runs_udf(slc_deployment: Deployment) -> None:
    """Custom install makes the container's UDFs runnable; remove cleans it up."""
    source_file = os.environ.get(CUSTOM_SLC_FILE_ENV)
    source_url = os.environ.get(CUSTOM_SLC_URL_ENV)
    if source_file:
        source_args = ["--file", source_file]
    elif source_url:
        source_args = ["--url", source_url]
    else:
        pytest.skip(
            f"set {CUSTOM_SLC_FILE_ENV} or {CUSTOM_SLC_URL_ENV} to a standard python "
            "container to run the custom SLC e2e"
        )

    _slc(
        slc_deployment,
        "custom",
        "install",
        *source_args,
        "--alias",
        CUSTOM_ALIAS,
        "--language",
        "python",
        "--auto-approve",
    )

    assert CUSTOM_ALIAS in _slc(slc_deployment, "list", capture=True).stdout
    assert "hi" in _run_scalar_udf(slc_deployment, CUSTOM_ALIAS, "slc_e2e_custom")

    _slc(slc_deployment, "custom", "remove", CUSTOM_ALIAS)
    assert CUSTOM_ALIAS not in _slc(slc_deployment, "list", capture=True).stdout
