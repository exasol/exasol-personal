# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

import json
import secrets
import subprocess
import time
from collections.abc import Iterator
from contextlib import contextmanager
from dataclasses import replace
from pathlib import Path
from typing import Any, Final

import pytest
import requests
import yaml

from framework.deployment import Deployment, DeploymentManager
from tests.testcase_helpers import container_command, host_command


class SidecarDeployment:
    def __init__(self, deployment: Deployment) -> None:
        self.deployment = deployment
        self.directory = Path(deployment.deployment_dir.name)
        state = json.loads(
            (self.directory / ".exasolLauncherState.json").read_text(encoding="utf-8")
        )
        self.database = "exasol-db-" + state["deploymentId"]
        self.container = self.database + "-sidecar-caddy"
        self.network = self.database + "-services"

    def sidecar(self, operation: str, *args: str) -> dict[str, Any]:
        result = self.deployment.launcher.run_command(
            "sidecar",
            str(self.directory),
            operation,
            "caddy",
            "--json",
            *args,
            capture_output=True,
            timeout=300,
        )
        return dict(json.loads(result.stdout))

    def host(self, *args: str, node: str | None = None) -> str:
        return host_command(
            self.deployment.launcher.launcher_path,
            self.directory,
            *args,
            node=node,
        ).stdout

    def podman(self, *args: str, node: str | None = None) -> str:
        return container_command(
            self.deployment.launcher.launcher_path,
            self.directory,
            *args,
            node=node,
        ).stdout

    def inspect(
        self, name: str | None = None, *, node: str | None = None
    ) -> dict[str, Any]:
        return dict(
            json.loads(self.podman("inspect", name or self.container, node=node))[0]
        )

    def definition(self) -> dict[str, Any]:
        return dict(
            yaml.safe_load(
                (self.directory / "sidecars.yaml").read_text(encoding="utf-8")
            )["containers"][0]
        )

    def save(self, *containers: dict[str, Any]) -> None:
        (self.directory / "sidecars.yaml").write_text(
            yaml.safe_dump({"version": 1, "containers": list(containers)}),
            encoding="utf-8",
        )

    def _state_path(self) -> Path:
        return self.directory / "sidecars-state.json"

    def record_failures(self, failures: dict[str, str]) -> None:
        """Set the recorded diagnostics, keeping the rest of the state intact."""
        path = self._state_path()
        state = json.loads(path.read_text(encoding="utf-8")) if path.exists() else {}
        state["failures"] = failures
        path.write_text(json.dumps(state), encoding="utf-8")

    def recorded_failures(self) -> dict[str, str]:
        path = self._state_path()
        if not path.exists():
            return {}

        return dict(json.loads(path.read_text(encoding="utf-8")).get("failures") or {})

    def response(self) -> str:
        endpoint = self.sidecar("status")["endpoints"][0]
        return self.external_response(f"http://{endpoint['ip']}:{endpoint['port']}")

    def external_response(self, url: str, *, timeout: float = 30) -> str:
        deadline = time.monotonic() + timeout
        with requests.Session() as session:
            session.trust_env = False
            while True:
                try:
                    response = session.get(url, timeout=5)
                    response.raise_for_status()
                except requests.RequestException:
                    if time.monotonic() >= deadline:
                        raise
                    time.sleep(1)
                else:
                    return response.text

    def sql(self, statement: str = "SELECT 42;") -> str:
        return self.deployment.connect(
            input=statement,
            capture_output=True,
            timeout=60,
        ).stdout

    def randomize_password(self) -> str:
        password = "Test" + secrets.token_hex(16)
        self.sql(f'ALTER USER sys IDENTIFIED BY "{password}";')
        path = self.directory / "secrets.json"
        values = json.loads(path.read_text(encoding="utf-8"))
        values["dbPassword"] = password
        path.write_text(json.dumps(values), encoding="utf-8")
        assert "42" in self.sql()
        return password

    def internal_response(
        self, url: str = "http://caddy:8080", *, node: str | None = None
    ) -> str:
        deadline = time.monotonic() + 30
        while True:
            try:
                response = self.podman(
                    "exec",
                    self.container,
                    "wget",
                    "-T",
                    "2",
                    "-qO-",
                    url,
                    node=node,
                )
            except subprocess.CalledProcessError:
                if time.monotonic() >= deadline:
                    raise
                time.sleep(0.2)
            else:
                return response

    def database_reachable(self) -> None:
        self.podman(
            "exec", self.container, "sh", "-c", "nc -w 2 database 8563 </dev/null"
        )


FIXTURE_CATALOG: Final = Path(__file__).parents[1] / "fixtures/sidecars/catalog.yaml"


def fixture_definition(name: str = "caddy") -> dict[str, Any]:
    """Return the fixture container definition, named for a saved document."""
    catalog = yaml.safe_load(FIXTURE_CATALOG.read_text(encoding="utf-8"))
    definition = dict(catalog["sidecars"][name]["container"])
    definition["name"] = name

    return definition


def _save_fixture(deployment: Deployment) -> None:
    SidecarDeployment(deployment).save(fixture_definition())


@contextmanager
def sidecar_deployment(
    manager: DeploymentManager,
    infra: str,
    *,
    running: bool = True,
) -> Iterator[SidecarDeployment]:
    if infra != "local":
        pytest.skip("sidecar deployment tests require local infrastructure")
    deployment = manager.local(deploy=False)
    try:
        if running:
            deployment.deploy()
        _save_fixture(deployment)
        yield SidecarDeployment(deployment)
    finally:
        manager.release(deployment)


@contextmanager
def concurrent_sidecar_deployments(
    manager: DeploymentManager,
    infra: str,
    count: int,
) -> Iterator[list[SidecarDeployment]]:
    """Own several local deployments at the same time.

    The deployment manager keeps a single local deployment, so deployment-scoped
    service names are exercised through directly owned instances instead.
    """
    if infra != "local":
        pytest.skip("sidecar deployment tests require local infrastructure")
    deployments: list[Deployment] = []
    try:
        for _ in range(count):
            deployment = Deployment(
                manager.launcher,
                "--no-launcher-version-check",
                config=replace(manager.config, infra="local", cluster_size=1),
            )
            deployments.append(deployment)
            _save_fixture(deployment)
        yield [SidecarDeployment(deployment) for deployment in deployments]
    finally:
        for deployment in deployments:
            deployment.cleanup()
