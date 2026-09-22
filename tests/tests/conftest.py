# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

import logging
import os
import platform
import re
import shlex
import subprocess
from collections.abc import Generator, Sequence
from pathlib import Path
from typing import Any, Protocol

import pytest
from _pytest.reports import TestReport
from _pytest.terminal import TerminalReporter

from framework.deployment import (
    Deployment,
    DeploymentManager,
)
from framework.launcher import DeploymentConfig, Launcher

_ACTIVE_PROVIDERS = ("aws", "azure", "exoscale", "local")
_ACTIVE_PLATFORMS = (
    "linux-amd64",
    "macos-amd64",
    "macos-arm64",
    "windows-amd64",
)
_EXECUTION_BOUNDARIES = (
    "controlled",
    "local",
    "shared-live",
    "reusable-live",
    "isolated-live",
)
_SPECIALIZED_SUITES = ("smoke", "chaos", "stress")
_LIVE_FIXTURES = (
    "local_deployment",
    "shared_live_deployment",
    "reusable_live_deployment",
    "isolated_live_deployment",
)
_CLOUD_OPT_IN = "EXASOL_RUN_CLOUD_DEPLOY_CASES"
_EVIDENCE_KEY = pytest.StashKey[dict[str, set[str]]]()
_DEPLOYMENTS_CREATED_KEY = pytest.StashKey[int]()
_REDACTED_VALUE = "<redacted>"
_SENSITIVE_FLAGS = ("--db-password", "--adminui-password", "--password")


class _WorkerNode(Protocol):
    config: pytest.Config
    workeroutput: dict[str, int]


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
        choices=_ACTIVE_PROVIDERS,
        help="Infrastructure preset to use for deployment tests",
    )
    parser.addoption(
        "--platform",
        choices=_ACTIVE_PLATFORMS,
        default=_host_platform(),
        help="Host platform used to select platform-restricted tests",
    )


@pytest.fixture(scope="session")
def exasol_path(request: pytest.FixtureRequest) -> str:
    return str(request.config.getoption("--exasol-path"))


@pytest.fixture(scope="session")
def infra(request: pytest.FixtureRequest) -> str:
    return str(request.config.getoption("--infra"))


@pytest.fixture(scope="session")
def deployment_manager(
    request: pytest.FixtureRequest, exasol_path: str, infra: str
) -> Generator[DeploymentManager]:
    cluster_size = 2 if infra == "aws" else 1
    config = DeploymentConfig(infra=infra, cluster_size=cluster_size)
    manager = DeploymentManager(Launcher(exasol_path), config)
    try:
        yield manager
    finally:
        try:
            manager.close()
        finally:
            _record_deployments_created(request.config, manager.created_count)


@pytest.fixture(scope="session")
def reusable_runtime_cache(tmp_path_factory: pytest.TempPathFactory) -> Path:
    return tmp_path_factory.mktemp("reusable-runtime-cache")


@pytest.fixture(scope="session")
def reusable_deployment_manager(
    request: pytest.FixtureRequest,
    exasol_path: str,
    infra: str,
    reusable_runtime_cache: Path,
) -> Generator[DeploymentManager]:
    cluster_size = 2 if infra == "aws" else 1
    config = DeploymentConfig(infra=infra, cluster_size=cluster_size)
    manager = DeploymentManager(Launcher(exasol_path), config)
    try:
        yield manager
    finally:
        try:
            with pytest.MonkeyPatch.context() as monkeypatch:
                monkeypatch.setenv("XDG_CACHE_HOME", str(reusable_runtime_cache))
                manager.close()
        finally:
            _record_deployments_created(request.config, manager.created_count)


@pytest.fixture
def deployment(deployment_manager: DeploymentManager) -> Generator[Deployment]:
    instance = deployment_manager.controlled()
    try:
        yield instance
    finally:
        deployment_manager.release(instance)


@pytest.fixture
def shared_live_deployment(
    deployment_manager: DeploymentManager,
) -> Generator[Deployment]:
    instance = deployment_manager.shared_live()
    yield instance
    deployment_manager.verify_shared_ready(instance)


@pytest.fixture
def reusable_live_deployment(
    reusable_deployment_manager: DeploymentManager,
    reusable_runtime_cache: Path,
) -> Generator[Deployment]:
    with pytest.MonkeyPatch.context() as monkeypatch:
        monkeypatch.setenv("XDG_CACHE_HOME", str(reusable_runtime_cache))
        instance = reusable_deployment_manager.reusable_live()
        try:
            yield instance
        finally:
            reusable_deployment_manager.restore_reusable_ready(instance)


@pytest.fixture
def isolated_live_deployment(
    deployment_manager: DeploymentManager,
) -> Generator[Deployment]:
    instance = deployment_manager.isolated_live()
    try:
        yield instance
    finally:
        deployment_manager.release(instance)


@pytest.fixture
def local_deployment(
    deployment_manager: DeploymentManager, infra: str
) -> Generator[Deployment]:
    if infra != "local":
        pytest.skip("local deployment requires --infra=local")
    instance = deployment_manager.local()
    try:
        yield instance
    finally:
        deployment_manager.release(instance)


# Every command the suite executes goes through subprocess, so logging one
# central place exposes them all -- the helpers in `launcher/helpers.py` and
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


@pytest.hookimpl(tryfirst=True)
def pytest_collection_modifyitems(
    config: pytest.Config, items: list[pytest.Item]
) -> None:
    selected_infra = str(config.getoption("--infra"))
    selected_platform = str(config.getoption("--platform"))
    selected_items: list[pytest.Item] = []
    deselected_items: list[pytest.Item] = []

    for item in items:
        fixture_names = item.fixturenames if isinstance(item, pytest.Function) else ()
        _validate_launcher_boundary(item, fixture_names)
        provider_infras = _marker_values(item, "providers", _ACTIVE_PROVIDERS)
        platforms = _marker_values(item, "platform", _ACTIVE_PLATFORMS)

        if (
            _requires_cloud_opt_in(item, selected_infra)
            and os.environ.get(_CLOUD_OPT_IN) != "1"
        ):
            item.add_marker(
                pytest.mark.skip(
                    reason=f"set {_CLOUD_OPT_IN}=1 to run live cloud tests"
                )
            )

        if group := _xdist_group(fixture_names, provider_infras, selected_infra):
            item.add_marker(pytest.mark.xdist_group(name=group))

        if (provider_infras and selected_infra not in provider_infras) or (
            platforms and selected_platform not in platforms
        ):
            deselected_items.append(item)
        else:
            boundary = _execution_boundary(
                fixture_names, provider_infras, selected_infra
            )
            item.user_properties.append(("evidence_boundary", boundary))
            item.user_properties.append(("evidence_platform", selected_platform))
            if boundary != "controlled":
                item.user_properties.append(("evidence_provider", selected_infra))
            for suite in _SPECIALIZED_SUITES:
                if item.get_closest_marker(suite):
                    item.user_properties.append(("evidence_suite", suite))
            selected_items.append(item)

    if deselected_items:
        config.hook.pytest_deselected(items=deselected_items)
        items[:] = selected_items


def pytest_collection_finish(session: pytest.Session) -> None:
    session.config.stash[_EVIDENCE_KEY] = _evidence_from_items(session.items)


@pytest.hookimpl(trylast=True)
def pytest_sessionfinish(session: pytest.Session, exitstatus: pytest.ExitCode) -> None:
    del exitstatus
    workeroutput = getattr(session.config, "workeroutput", None)
    if workeroutput is not None:
        workeroutput["deployments_created"] = session.config.stash.get(
            _DEPLOYMENTS_CREATED_KEY, 0
        )


def pytest_testnodedown(node: _WorkerNode, error: object | None) -> None:
    del error
    if not hasattr(node, "workeroutput"):
        return
    _record_deployments_created(
        node.config,
        node.workeroutput.get("deployments_created", 0),
    )


def pytest_terminal_summary(
    terminalreporter: TerminalReporter,
    exitstatus: pytest.ExitCode,
    config: pytest.Config,
) -> None:
    del exitstatus
    evidence = config.stash.get(_EVIDENCE_KEY, _empty_evidence())
    for reports in terminalreporter.stats.values():
        for report in reports:
            if isinstance(report, TestReport):
                _add_evidence_properties(evidence, report.user_properties)

    lines = _evidence_summary(evidence, config.stash.get(_DEPLOYMENTS_CREATED_KEY, 0))
    terminalreporter.section("test evidence")
    for line in lines:
        terminalreporter.write_line(line)

    if (summary_path := os.environ.get("GITHUB_STEP_SUMMARY")) and not hasattr(
        config, "workerinput"
    ):
        with Path(summary_path).open("a", encoding="utf-8") as summary:
            summary.write("### Test evidence\n\n")
            summary.writelines(f"- {line}\n" for line in lines)


def pytest_report_header(config: pytest.Config) -> list[str]:
    return [
        (
            f"test target: provider={config.getoption('--infra')}, "
            f"platform={config.getoption('--platform')}"
        ),
        "provider coverage: STACKIT omitted because its deployment path is unavailable",
    ]


def _marker_values(
    item: pytest.Item, marker_name: str, allowed: Sequence[str]
) -> set[str]:
    marker = item.get_closest_marker(marker_name)
    if marker is None:
        return set()
    values = {str(value) for value in marker.args}
    unknown = values.difference(allowed)
    if unknown:
        message = f"Unknown {marker_name}: {', '.join(sorted(unknown))}"
        raise pytest.UsageError(message)
    return values


def _validate_launcher_boundary(
    item: pytest.Item, fixture_names: Sequence[str]
) -> None:
    live_fixtures = sorted(set(fixture_names).intersection(_LIVE_FIXTURES))
    if "launcher" in item.path.parts and live_fixtures:
        message = (
            f"Launcher tests cannot request live fixtures: {', '.join(live_fixtures)}"
        )
        raise pytest.UsageError(message)


def _requires_cloud_opt_in(item: pytest.Item, selected_infra: str) -> bool:
    return selected_infra != "local" and "live" in item.path.parts


def _xdist_group(
    fixture_names: Sequence[str], provider_infras: set[str], selected_infra: str
) -> str | None:
    managed_live = (
        any(
            manager in fixture_names
            for manager in ("deployment_manager", "reusable_deployment_manager")
        )
        and "deployment" not in fixture_names
    )
    if selected_infra == "local" and (managed_live or "local" in provider_infras):
        return "local-live"
    if "shared_live_deployment" in fixture_names:
        return "shared-live"
    if "reusable_live_deployment" in fixture_names:
        return "reusable-live"
    return None


def _execution_boundary(
    fixture_names: Sequence[str], provider_infras: set[str], selected_infra: str
) -> str:
    if "deployment" in fixture_names:
        return "controlled"
    managed_live = any(
        manager in fixture_names
        for manager in ("deployment_manager", "reusable_deployment_manager")
    )
    if selected_infra == "local" and (managed_live or "local" in provider_infras):
        return "local"
    if "shared_live_deployment" in fixture_names:
        return "shared-live"
    if "reusable_live_deployment" in fixture_names:
        return "reusable-live"
    if managed_live:
        return "isolated-live"
    return "controlled"


def _empty_evidence() -> dict[str, set[str]]:
    return {name: set() for name in ("boundary", "provider", "platform", "suite")}


def _add_evidence_properties(
    evidence: dict[str, set[str]], properties: Sequence[tuple[str, object]]
) -> None:
    for name, value in properties:
        if name.startswith("evidence_"):
            evidence[name.removeprefix("evidence_")].add(str(value))


def _evidence_from_items(items: Sequence[pytest.Item]) -> dict[str, set[str]]:
    evidence = _empty_evidence()
    for item in items:
        _add_evidence_properties(evidence, item.user_properties)
    return evidence


def _evidence_summary(
    evidence: dict[str, set[str]], deployments_created: int = 0
) -> list[str]:
    categories = (
        ("execution boundaries", "boundary", _EXECUTION_BOUNDARIES),
        ("providers", "provider", _ACTIVE_PROVIDERS),
        ("platforms", "platform", _ACTIVE_PLATFORMS),
        ("specialized suites", "suite", _SPECIALIZED_SUITES),
    )
    lines: list[str] = []
    for label, key, available in categories:
        selected = evidence[key]
        lines.append(f"{label} selected: {_display_values(selected)}")
        lines.append(
            f"{label} omitted: {_display_values(set(available).difference(selected))}"
        )
    lines.append(
        "STACKIT omitted: its deployment path is not currently working correctly"
    )
    lines.append(f"deployments created: {deployments_created}")
    return lines


def _record_deployments_created(config: pytest.Config, count: int) -> None:
    config.stash[_DEPLOYMENTS_CREATED_KEY] = (
        config.stash.get(_DEPLOYMENTS_CREATED_KEY, 0) + count
    )


def _display_values(values: set[str]) -> str:
    return ", ".join(sorted(values)) or "none"


def _host_platform() -> str:
    system = platform.system().lower()
    machine = platform.machine().lower()
    if system == "darwin" and machine in {"arm64", "aarch64"}:
        return "macos-arm64"
    if system == "darwin":
        return "macos-amd64"
    if system == "windows":
        return "windows-amd64"
    return "linux-amd64"
