# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

import logging
import re
import shlex
import subprocess
from collections.abc import Generator, Sequence
from typing import Any

import pytest

from framework.deployment import Deployment, StatusDatabaseReady
from framework.launcher import DeploymentConfig, Launcher

_PROVIDER_MARKERS = {
    "provider_aws": "aws",
    "provider_azure": "azure",
    "provider_stackit": "stackit",
}

# Test-kind markers stamped onto every test based on the directory it lives in.
# The directory a test lives in is the single source of truth for its kind.
_KIND_MARKERS = ("integration", "deployment", "e2e", "chaos")
_REDACTED_VALUE = "<redacted>"
_SENSITIVE_FLAGS = ("--db-password", "--adminui-password", "--password")


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


# Every command the suite executes goes through subprocess, so logging one
# central place exposes them all -- the helpers in `integration/helpers.py` and
# `testcase_helpers.py`, the `framework` launcher, and the tests that reach for
# `subprocess` directly. `subprocess.run` builds on `Popen`, so patching only
# `Popen` covers both without logging the same command twice.
_command_logger = logging.getLogger("tests.commands")
_real_popen = subprocess.Popen


def _format_command(args: object) -> str:
    if isinstance(args, (str, bytes)):
        return _redact_command_string(
            args.decode() if isinstance(args, bytes) else args
        )
    if isinstance(args, (list, tuple)):
        return shlex.join(_redact_command_parts(args))
    return str(args)


def _redact_command_parts(args: Sequence[object]) -> list[str]:
    redacted_parts: list[str] = []
    redact_next = False
    for part in args:
        text = part.decode() if isinstance(part, bytes) else str(part)
        if redact_next:
            redacted_parts.append(_REDACTED_VALUE)
            redact_next = False
            continue
        if text in _SENSITIVE_FLAGS:
            redacted_parts.append(text)
            redact_next = True
            continue
        for flag in _SENSITIVE_FLAGS:
            if text.startswith(f"{flag}="):
                redacted_parts.append(f"{flag}={_REDACTED_VALUE}")
                break
        else:
            redacted_parts.append(text)
    return redacted_parts


def _redact_command_string(command: str) -> str:
    redacted = command
    for flag in _SENSITIVE_FLAGS:
        redacted = re.sub(
            rf"({re.escape(flag)})(\s+|=)(?P<value>(?:'[^']*'|\"[^\"]*\"|\S+))",
            rf"\1\2{_REDACTED_VALUE}",
            redacted,
        )
    return redacted


def _logging_popen(
    args: Any,  # noqa: ANN401
    *rest: Any,  # noqa: ANN401
    **kwargs: Any,  # noqa: ANN401
) -> subprocess.Popen[Any]:
    """Log the command line at DEBUG, then start it through the real ``Popen``."""
    # The working directory is logged when set, because several tests rely on
    # resolution relative to the cwd; the environment is not, since dumping it
    # would bury the command line.
    cwd = kwargs.get("cwd")
    context = f" (cwd: {cwd})" if cwd is not None else ""
    _command_logger.debug("executing command%s: %s", context, _format_command(args))
    return _real_popen(args, *rest, **kwargs)


# pytest passes only the hook arguments a hook actually declares, so both of
# these take none.
def pytest_configure() -> None:
    subprocess.Popen = _logging_popen  # type: ignore[assignment,misc]


def pytest_unconfigure() -> None:
    subprocess.Popen = _real_popen  # type: ignore[misc]


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
