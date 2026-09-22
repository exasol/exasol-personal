# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT
# ruff: noqa: INP001

from __future__ import annotations

import itertools
import logging
import platform as host_platform
import random
import time
from dataclasses import dataclass
from typing import TYPE_CHECKING

import pytest

if TYPE_CHECKING:
    from collections.abc import Iterator

ACTIVE_PROVIDERS = ("aws", "azure", "exoscale", "local")
ACTIVE_PLATFORMS = ("linux-amd64", "macos-arm64", "windows-amd64")


@dataclass
class FakeDeployment:
    deployment_id: str
    provider: str
    boundary: str
    state: str = "ready"

    def query(self, statement: str) -> int:
        assert self.state == "ready"
        logging.info("[deployment %s] query: %s", self.deployment_id, statement)
        return 1

    def stop(self) -> None:
        assert self.state == "ready"
        self.state = "stopped"
        logging.info("[deployment %s] stopped", self.deployment_id)

    def start(self) -> None:
        assert self.state == "stopped"
        self.state = "ready"
        logging.info("[deployment %s] ready", self.deployment_id)


class DeploymentManager:
    def __init__(self, provider: str, worker_id: str) -> None:
        self.provider = provider
        self.worker_id = worker_id
        self._ids = itertools.count(1)
        self._random = random.Random(32218)  # noqa: S311
        self._shared: FakeDeployment | None = None
        self._owned: dict[str, FakeDeployment] = {}

    def shared(self) -> FakeDeployment:
        if self._shared is None:
            self._shared = self._create("shared-live")
        else:
            logging.info("[manager] reusing %s", self._shared.deployment_id)
        return self._shared

    def isolated(self) -> FakeDeployment:
        return self._create("isolated-live")

    def local(self) -> FakeDeployment:
        return self._create("local")

    def controlled(self) -> FakeDeployment:
        return self._create("controlled")

    def release(self, deployment: FakeDeployment) -> None:
        if self._owned.pop(deployment.deployment_id, None) is None:
            return
        self._pause("cleanup", deployment)
        deployment.state = "removed"

    def close(self) -> None:
        for deployment in tuple(self._owned.values()):
            self.release(deployment)

    def _create(self, boundary: str) -> FakeDeployment:
        deployment = FakeDeployment(
            deployment_id=f"{self.worker_id}-demo-{next(self._ids)}",
            provider=self.provider,
            boundary=boundary,
        )
        self._owned[deployment.deployment_id] = deployment
        self._pause("create", deployment)
        return deployment

    def _pause(self, action: str, deployment: FakeDeployment) -> None:
        delay = self._random.uniform(0.05, 0.15)
        logging.info(
            "[manager] %s %s deployment %s for %s (%.2fs)",
            action,
            deployment.boundary,
            deployment.deployment_id,
            deployment.provider,
            delay,
        )
        time.sleep(delay)


def pytest_addoption(parser: pytest.Parser) -> None:
    parser.addoption(
        "--provider",
        choices=ACTIVE_PROVIDERS,
        default="aws",
        help="Provider simulated by the test-framework prototype",
    )
    parser.addoption(
        "--platform",
        choices=ACTIVE_PLATFORMS,
        default=_default_platform(),
        help="Platform simulated by the test-framework prototype",
    )


def pytest_configure(config: pytest.Config) -> None:
    markers = (
        "openspec(*capabilities): related OpenSpec capability IDs",
        "smoke: critical-path provider coverage",
        "providers(*names): supported providers for this test",
        "platform(*names): supported platforms for this test",
        "chaos: disruptive recovery or fault behavior",
        "stress: intentionally expensive stability or load coverage",
    )
    for marker in markers:
        config.addinivalue_line("markers", marker)


@pytest.hookimpl(tryfirst=True)
def pytest_collection_modifyitems(
    config: pytest.Config, items: list[pytest.Item]
) -> None:
    provider = str(config.getoption("--provider"))
    selected_platform = str(config.getoption("--platform"))

    for item in items:
        _skip_when_not_selected(item, "providers", provider)
        _skip_when_not_selected(item, "platform", selected_platform)
        if "shared_live_deployment" in item.fixturenames:
            item.add_marker(pytest.mark.xdist_group(name="shared-live"))


def pytest_report_header(config: pytest.Config) -> list[str]:
    return [
        (
            "test-framework prototype: "
            f"provider={config.getoption('--provider')}, "
            f"platform={config.getoption('--platform')}"
        ),
        "provider coverage: STACKIT omitted because its deployment path is unavailable",
    ]


@pytest.fixture(scope="session")
def deployment_manager(
    pytestconfig: pytest.Config, worker_id: str
) -> Iterator[DeploymentManager]:
    manager = DeploymentManager(str(pytestconfig.getoption("--provider")), worker_id)
    yield manager
    manager.close()


@pytest.fixture
def deployment(deployment_manager: DeploymentManager) -> Iterator[FakeDeployment]:
    instance = deployment_manager.controlled()
    yield instance
    deployment_manager.release(instance)


@pytest.fixture
def shared_live_deployment(
    deployment_manager: DeploymentManager,
) -> Iterator[FakeDeployment]:
    instance = deployment_manager.shared()
    yield instance
    message = (
        f"shared deployment {instance.deployment_id} "
        f"was left in {instance.state!r} state"
    )
    assert instance.state == "ready", message


@pytest.fixture
def isolated_live_deployment(
    deployment_manager: DeploymentManager,
) -> Iterator[FakeDeployment]:
    instance = deployment_manager.isolated()
    yield instance
    deployment_manager.release(instance)


@pytest.fixture
def local_deployment(
    deployment_manager: DeploymentManager,
) -> Iterator[FakeDeployment]:
    instance = deployment_manager.local()
    yield instance
    deployment_manager.release(instance)


def _skip_when_not_selected(item: pytest.Item, marker_name: str, selected: str) -> None:
    marker = item.get_closest_marker(marker_name)
    if marker is not None and selected not in marker.args:
        item.add_marker(
            pytest.mark.skip(reason=f"{marker_name} restriction excludes {selected}")
        )


def _default_platform() -> str:
    system = host_platform.system().lower()
    machine = host_platform.machine().lower()
    if system == "darwin" and machine in {"arm64", "aarch64"}:
        return "macos-arm64"
    if system == "windows":
        return "windows-amd64"
    return "linux-amd64"
