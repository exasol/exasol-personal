# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

import logging
import platform
import subprocess
import time
from collections.abc import Generator

import pytest

from framework.deployment import (
    Deployment,
    StatusDatabaseReady,
    StatusOperationInProgress,
)
from framework.launcher import DeploymentConfig, Launcher

_PROVIDER_MARKERS = {
    "provider_aws": "aws",
    "provider_azure": "azure",
    "provider_stackit": "stackit",
}

# Test-kind markers stamped onto every test based on the directory it lives in.
# The directory a test lives in is the single source of truth for its kind.
_KIND_MARKERS = ("integration", "deployment", "e2e", "chaos")


def pytest_addoption(parser: pytest.Parser) -> None:
    parser.addoption(
        "--exasol-path",
        type=str,
        required=False,
        action="store",
        default="exasol",
        help="Path to the exasol binary",
    )
    parser.addoption(
        "--infra",
        type=str,
        required=False,
        action="store",
        default="aws",
        choices=["aws", "azure", "exoscale", "stackit", "local"],
        help="Infrastructure preset to use for deployment tests",
    )
    parser.addoption(
        "--stackit-project-id",
        type=str,
        required=False,
        action="store",
        default=None,
        help="STACKIT project ID to put resources into",
    )


@pytest.fixture(scope="session")
def exasol_path(request: pytest.FixtureRequest) -> str:
    return str(request.config.getoption("--exasol-path"))


@pytest.fixture(scope="session")
def infra(request: pytest.FixtureRequest) -> str:
    return str(request.config.getoption("--infra"))


@pytest.fixture(scope="session")
def stackit_project_id(request: pytest.FixtureRequest) -> str | None:
    project_id = request.config.getoption("--stackit-project-id")
    if project_id is not None:
        return str(project_id)
    return None


@pytest.fixture(scope="session")
def reusable_deployment(
    exasol_path: str, infra: str, stackit_project_id: str | None
) -> Generator[Deployment]:
    """Session-scoped deployment shared by the deployment, e2e, and chaos suites.

    A single cluster is deployed once and reused across all cloud tests. Stateful
    tests (lifecycle, faults) must leave the deployment in a database-ready state so
    the read-only e2e tests can run regardless of directory ordering.
    """
    cluster_size = 2 if infra == "aws" else 1
    config = DeploymentConfig(
        infra=infra, cluster_size=cluster_size, stackit_project_id=stackit_project_id
    )
    deployment = Deployment(Launcher(exasol_path), config=config)
    try:
        deployment_proc = deployment.deploy_no_block()

        # Sleep after Popen to allow the child process to start
        # and update status (needed for Windows).
        if platform.system() == "Windows":
            time.sleep(3)

        logging.info("Check status deployment in progress")
        timeout = 10
        start_time = time.time()
        while True:
            if deployment.has_status(StatusOperationInProgress):
                break

            if time.time() - start_time > timeout:
                logging.info("Timeout expired. Status incorrect")
                msg = f"Expected status `{StatusOperationInProgress}` after `deploy`"
                raise RuntimeError(msg)

            logging.info("Status incorrect. Retrying in 5 seconds")
            time.sleep(5)

        logging.info("Waiting for deploy to complete")
        deploy_timeout = 40 * 60

        try:
            deploy_return_code = deployment_proc.wait(timeout=deploy_timeout)
        except subprocess.TimeoutExpired:
            deployment_proc.kill()
            deployment_proc.wait()
            msg = (
                f"Deploy command timed out after {deploy_timeout}s\n"
                f"deployment.log tail:\n{deployment.deployment_log_tail()}"
            )
            raise RuntimeError(msg) from None

        if deploy_return_code != 0:
            msg = (
                f"Deploy command failed with code {deploy_return_code}\n"
                f"deployment.log tail:\n{deployment.deployment_log_tail()}"
            )
            raise RuntimeError(msg)

        logging.info("Checking status database available")

        if not deployment.has_status(StatusDatabaseReady):
            msg = f"Expected status `{StatusDatabaseReady}` after `deploy`"
            raise RuntimeError(msg)

        yield deployment

    finally:
        deployment.cleanup()


def _stamp_kind_marker(item: pytest.Item) -> None:
    """Add the test-kind marker matching the directory the test lives in."""
    parts = item.path.parts
    for kind in _KIND_MARKERS:
        if kind in parts:
            item.add_marker(getattr(pytest.mark, kind))
            return


def pytest_collection_modifyitems(
    config: pytest.Config, items: list[pytest.Item]
) -> None:
    selected_infra = str(config.getoption("--infra"))
    selected_items: list[pytest.Item] = []
    deselected_items: list[pytest.Item] = []

    for item in items:
        _stamp_kind_marker(item)
        provider_infras = {
            infra
            for marker_name, infra in _PROVIDER_MARKERS.items()
            if item.get_closest_marker(marker_name) is not None
        }
        if provider_infras and selected_infra not in provider_infras:
            deselected_items.append(item)
        else:
            selected_items.append(item)

    if deselected_items:
        config.hook.pytest_deselected(items=deselected_items)
        items[:] = selected_items
