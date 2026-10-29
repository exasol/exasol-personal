# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

import json
import shutil
import socket
import subprocess
import sys
import time
from collections.abc import Iterator
from subprocess import CalledProcessError
from typing import Any

import pytest
import yaml

from framework.deployment import DeploymentManager, StatusDatabaseReady
from framework.sidecars import (
    SidecarDeployment,
    concurrent_sidecar_deployments,
    sidecar_deployment,
)
from tests.testcase_helpers import container_engine

# The network every container joins unless told otherwise.
DEFAULT_NETWORKS = {"docker": "bridge", "podman": "podman"}

pytestmark = [
    pytest.mark.openspec(
        "sidecar-configuration", "sidecar-lifecycle", "sidecar-networking"
    ),
    pytest.mark.providers("local"),
    # Windows reaches containers through a host shell the local runtime does
    # not provide, so these cannot run there yet.
    pytest.mark.platform("linux-amd64", "macos-arm64"),
]


@pytest.fixture
def service(
    deployment_manager: DeploymentManager, infra: str
) -> Iterator[SidecarDeployment]:
    with sidecar_deployment(deployment_manager, infra) as deployment:
        yield deployment


def test_sidecar_environment_values(service: SidecarDeployment) -> None:
    # Given
    password = service.randomize_password()
    # When
    service.sidecar("enable")
    # Then
    assert service.response() == f"database|8563|sys|{password}|initial-marker"
    environment = dict(
        value.split("=", 1) for value in service.inspect()["Config"]["Env"]
    )
    assert (
        environment.items()
        >= {
            "DB_HOST": "database",
            "DB_PORT": "8563",
            "DB_USER": "sys",
            "DB_PASSWORD": password,
            "TEST_MARKER": "initial-marker",
        }.items()
    )
    assert password not in (service.directory / "sidecars.yaml").read_text()


def test_sidecar_edited_definition_drops_password(service: SidecarDeployment) -> None:
    # Given
    service.sidecar("enable")
    definition = service.definition()
    definition["env"] = [v for v in definition["env"] if v["name"] != "DB_PASSWORD"]
    # When
    service.save(definition)
    service.deployment.start()
    # Then
    assert not any(
        value.startswith("DB_PASSWORD=")
        for value in service.podman("exec", service.container, "env").splitlines()
    )


def test_sidecar_rotated_credentials(service: SidecarDeployment) -> None:
    # Given
    service.sidecar("enable")
    password = service.randomize_password()
    # When
    service.deployment.start()
    # Then
    assert service.response() == f"database|8563|sys|{password}|initial-marker"


def test_sidecar_enable_idempotent(service: SidecarDeployment) -> None:
    # Given
    service.sidecar("enable")
    original = service.inspect()["Id"]
    assert service.response().endswith("|initial-marker")
    # When
    service.sidecar("enable")
    # Then
    assert service.inspect()["Id"] == original
    assert service.sidecar("status")["running"]
    assert service.response().endswith("|initial-marker")


@pytest.mark.parametrize("initial_state", ["initialized", "stopped"])
def test_sidecar_deferred_enable(
    deployment_manager: DeploymentManager,
    infra: str,
    initial_state: str,
) -> None:
    # Given
    with sidecar_deployment(deployment_manager, infra, running=False) as service:
        if initial_state == "stopped":
            service.deployment.deploy()
            service.deployment.stop()
        # When
        status = service.sidecar("enable")
        # Then
        assert status["enabled"]
        assert not status["running"]
        # When
        if initial_state == "initialized":
            service.deployment.deploy()
        else:
            service.deployment.start()
        # Then
        assert service.sidecar("status")["running"]
        assert service.response().endswith("|initial-marker")


def test_sidecar_start_reconciles_marker(service: SidecarDeployment) -> None:
    # Given
    service.sidecar("enable")
    definition = service.definition()
    next(v for v in definition["env"] if v["name"] == "TEST_MARKER")["value"] = "edited"
    # When
    service.save(definition)
    service.deployment.start()
    # Then
    assert service.response().endswith("|edited")
    assert service.deployment.status_value() == StatusDatabaseReady
    assert "42" in service.sql()


def test_sidecar_start_preserves_container(service: SidecarDeployment) -> None:
    # Given
    service.sidecar("enable")
    original = service.inspect()["Id"]
    # When
    service.deployment.start()
    # Then
    assert service.inspect()["Id"] == original
    assert service.response().endswith("|initial-marker")


def test_sidecar_status_reads_runtime_observations(service: SidecarDeployment) -> None:
    # Given
    service.sidecar("enable")
    original = service.inspect()["Id"]
    definition = service.definition()
    failure = {"caddy": "previous publication failure"}
    service.record_failures(failure)
    # When
    current = service.sidecar("status")
    # Then
    assert current["reconciliation"] == "complete"
    assert "lastOperationError" not in current
    assert service.recorded_failures() == {}
    assert service.inspect()["Id"] == original
    # When
    edited = service.definition()
    next(v for v in edited["env"] if v["name"] == "TEST_MARKER")["value"] = "pending"
    service.save(edited)
    service.record_failures(failure)
    pending = service.sidecar("status")
    # Then
    assert pending["running"]
    assert pending["reconciliation"] == "pending"
    assert pending["lastOperationError"] == failure["caddy"]
    assert service.inspect()["Id"] == original
    assert service.response().endswith("|initial-marker")
    # When
    service.save(definition)
    restored = service.sidecar("status")
    # Then
    assert restored["reconciliation"] == "complete"
    assert "lastOperationError" not in restored
    assert service.inspect()["Id"] == original
    # When
    service.sidecar("disable")
    service.record_failures(failure)
    removed = service.sidecar("status")
    # Then
    assert removed["reconciliation"] == "complete"
    assert "lastOperationError" not in removed
    assert service.recorded_failures() == {}


def test_sidecar_start_removes_deleted_definition(service: SidecarDeployment) -> None:
    # Given
    service.sidecar("enable")
    # When
    service.save()
    service.deployment.start()
    # Then
    statuses = json.loads(
        service.deployment.launcher.run_command(
            "sidecar",
            str(service.directory),
            "status",
            "--json",
            capture_output=True,
        ).stdout
    )
    assert not any(status["name"] == "caddy" for status in statuses)
    assert (
        service.podman(
            "ps",
            "-a",
            "--filter",
            f"name={service.container}",
            "--format",
            "{{.Names}}",
        ).strip()
        == ""
    )
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as listener:
        listener.bind(("127.0.0.1", 18080))


def test_sidecar_deployment_restart(service: SidecarDeployment) -> None:
    # Given
    service.sidecar("enable")
    # When
    service.deployment.stop()
    stopped = service.sidecar("status")
    service.deployment.start()
    # Then
    assert stopped["enabled"]
    assert not stopped["running"]
    assert service.response().endswith("|initial-marker")


def test_sidecar_disable_idempotent(service: SidecarDeployment) -> None:
    # Given
    service.sidecar("enable")
    service.sql(
        "CREATE SCHEMA SIDECAR_TEST; CREATE TABLE SIDECAR_TEST.T (N INT); "
        "INSERT INTO SIDECAR_TEST.T VALUES (42); COMMIT;"
    )
    database = service.inspect(service.database)["Id"]
    # When
    first = service.sidecar("disable")
    second = service.sidecar("disable")
    # Then
    assert "nothingToRemove" not in first
    assert second["nothingToRemove"] is True
    assert not second["enabled"]
    assert not second["running"]
    assert service.inspect(service.database)["Id"] == database
    assert "42" in service.sql("SELECT N FROM SIDECAR_TEST.T;")


def _owned_containers(service: SidecarDeployment) -> list[str]:
    return service.podman(
        "ps", "-a", "--filter", f"name={service.container}", "--format", "{{.Names}}"
    ).split()


def _saved_entry(service: SidecarDeployment) -> dict[str, object]:
    path = service.directory / "sidecars.yaml"
    return dict(yaml.safe_load(path.read_text(encoding="utf-8"))["sidecars"]["caddy"])


def test_sidecar_reenable_restores_edited_definition(
    service: SidecarDeployment,
) -> None:
    # Given
    service.sidecar("enable")
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as probe:
        probe.bind(("127.0.0.1", 0))
        edited_port = int(probe.getsockname()[1])
    definition = service.definition()
    definition["ports"][0]["hostPort"] = edited_port
    service.save(definition)
    # When
    disabled = service.sidecar("disable")
    remaining = _owned_containers(service)
    saved = _saved_entry(service)
    enabled = service.sidecar("enable")
    # Then
    assert not disabled["running"]
    assert remaining == []
    assert saved["enabled"] is False
    assert saved["container"]["ports"][0]["hostPort"] == edited_port  # type: ignore[index]
    assert enabled["endpoints"] == [{"ip": "127.0.0.1", "port": edited_port}]
    assert service.response().endswith("|initial-marker")


def test_sidecar_start_removes_disabled_definition(service: SidecarDeployment) -> None:
    # Given
    service.sidecar("enable")
    path = service.directory / "sidecars.yaml"
    document = yaml.safe_load(path.read_text(encoding="utf-8"))
    document["sidecars"]["caddy"]["enabled"] = False
    path.write_text(yaml.safe_dump(document), encoding="utf-8")
    # When
    service.deployment.start()
    status = service.sidecar("status")
    # Then
    assert not status["enabled"]
    assert not status["running"]
    assert status["reconciliation"] == "complete"
    assert _owned_containers(service) == []
    assert "container" in _saved_entry(service)
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as listener:
        listener.bind(("127.0.0.1", 18080))


def test_sidecar_status_lifecycle(service: SidecarDeployment) -> None:
    # Given
    service.sidecar("enable")
    # When
    running = service.sidecar("status")
    response = service.response()
    text = service.deployment.launcher.run_command(
        "sidecar",
        str(service.directory),
        "status",
        "caddy",
        capture_output=True,
    ).stdout
    service.deployment.stop()
    stopped = service.sidecar("status")
    stopped_text = service.deployment.launcher.run_command(
        "sidecar",
        str(service.directory),
        "status",
        "caddy",
        capture_output=True,
    ).stdout
    # Then
    assert running["enabled"]
    assert running["running"]
    assert "caddy: enabled" in text
    assert "State: running" in text
    assert running["endpoints"] == [{"ip": "127.0.0.1", "port": 18080}]
    assert response.endswith("|initial-marker")
    assert "127.0.0.1:18080" in text
    assert stopped["enabled"]
    assert not stopped["running"]
    assert stopped["endpoints"] == []
    assert "caddy: enabled" in stopped_text
    assert "State: stopped" in stopped_text


def test_sidecar_failure_preserves_database_status(service: SidecarDeployment) -> None:
    # Given
    service.sidecar("enable")
    definition = service.definition()
    definition["command"] = ["/missing-sidecar-program"]
    service.save(definition)
    # When
    with pytest.raises(CalledProcessError):
        service.deployment.start()
    # Then
    assert service.deployment.status_value() == StatusDatabaseReady
    assert service.sidecar("status")["lastOperationError"]
    assert "42" in service.sql()


@pytest.mark.parametrize(
    ("policy", "exit_code", "restarts"),
    [
        ("Always", 0, True),
        ("Always", 1, True),
        ("OnFailure", 0, False),
        ("OnFailure", 1, True),
        ("Never", 0, False),
        ("Never", 1, False),
    ],
)
def test_sidecar_process_restart_policy(
    service: SidecarDeployment,
    policy: str,
    exit_code: int,
    *,
    restarts: bool,
) -> None:
    # Given
    service.sidecar("enable")
    definition = service.definition()
    definition["restartPolicy"] = policy
    definition["command"] = ["sh", "-c"]
    definition["args"] = [
        (
            "while [ ! -f /tmp/sidecar-exit ]; do sleep 0.1; done; "
            'code=$(cat /tmp/sidecar-exit); rm /tmp/sidecar-exit; exit "$code"'
        )
    ]
    service.save(definition)
    service.deployment.start()
    started = service.inspect()["State"]["StartedAt"]
    # When
    service.podman(
        "exec",
        "--detach",
        service.container,
        "sh",
        "-c",
        f"echo {exit_code} > /tmp/sidecar-exit",
    )
    deadline = time.monotonic() + 30
    while time.monotonic() < deadline:
        observed = service.inspect()
        if (
            restarts
            and observed["State"]["Running"]
            and observed["State"]["StartedAt"] != started
        ):
            break
        if not restarts and not observed["State"]["Running"]:
            break
        time.sleep(0.2)
    # Then
    assert observed["State"]["Running"] == restarts
    if restarts:
        assert observed["State"]["StartedAt"] != started
    else:
        assert observed["State"]["ExitCode"] == exit_code
        time.sleep(2)
        assert not service.inspect()["State"]["Running"]


def _save_probe(service: SidecarDeployment, probe: dict[str, Any]) -> None:
    definition = service.definition()
    definition["readinessProbe"] = probe
    service.save(definition)


def _status_until(
    service: SidecarDeployment, health: str, *, timeout: float = 60
) -> dict[str, Any]:
    deadline = time.monotonic() + timeout
    while True:
        status = service.sidecar("status")
        if status["health"] == health or time.monotonic() >= deadline:
            return status
        time.sleep(1)


def test_sidecar_health_serving(service: SidecarDeployment) -> None:
    # Given
    _save_probe(service, {"httpGet": {"port": "http", "path": "/"}})
    service.sidecar("enable")
    # When
    status = _status_until(service, "serving")
    text = service.deployment.launcher.run_command(
        "sidecar", str(service.directory), "status", "caddy", capture_output=True
    ).stdout
    # Then
    assert status["health"] == "serving"
    assert "healthError" not in status
    assert "Health: serving" in text


def test_sidecar_health_starting(service: SidecarDeployment) -> None:
    # Given
    _save_probe(service, {"exec": {"command": ["false"]}, "periodSeconds": 600})
    service.sidecar("enable", "--no-wait")
    # When
    status = service.sidecar("status")
    # Then
    assert status["health"] == "starting"
    assert status["healthError"]


def test_sidecar_health_failing(service: SidecarDeployment) -> None:
    # Given
    _save_probe(
        service,
        {"exec": {"command": ["false"]}, "periodSeconds": 1, "failureThreshold": 1},
    )
    service.sidecar("enable")
    # When
    status = _status_until(service, "failing", timeout=30)
    # Then
    assert status["health"] == "failing"
    assert status["healthError"]
    assert status["state"] == "running"
    assert service.inspect()["State"]["Running"]


def test_sidecar_without_probes_reports_no_health(service: SidecarDeployment) -> None:
    # Given
    service.sidecar("enable")
    # When
    status = service.sidecar("status")
    # Then
    assert status["running"]
    assert status["health"] == ""
    assert "healthError" not in status


def test_sidecar_edited_probe_keeps_container(service: SidecarDeployment) -> None:
    # Given
    service.sidecar("enable")
    original = service.inspect()["Id"]
    # When
    _save_probe(service, {"tcpSocket": {"port": 8080}})
    status = _status_until(service, "serving")
    # Then
    assert status["health"] == "serving"
    assert status["reconciliation"] == "complete"
    assert service.inspect()["Id"] == original


def _enable_text(service: SidecarDeployment, *args: str) -> str:
    return str(
        service.deployment.launcher.run_command(
            "sidecar",
            str(service.directory),
            "enable",
            "caddy",
            *args,
            capture_output=True,
        ).stderr
    )


def test_sidecar_enable_returns_once_serving(service: SidecarDeployment) -> None:
    # Given
    _save_probe(service, {"httpGet": {"port": "http"}, "initialDelaySeconds": 5})
    # When
    enabled = service.sidecar("enable")
    # Then
    assert enabled["health"] == "serving"


def test_sidecar_enable_reports_never_serving(service: SidecarDeployment) -> None:
    # Given
    window = 3
    _save_probe(
        service,
        {
            "exec": {"command": ["false"]},
            "periodSeconds": 1,
            "failureThreshold": window,
        },
    )
    # When
    started = time.monotonic()
    enabled = service.sidecar("enable")
    waited = time.monotonic() - started
    guidance = _enable_text(service)
    # Then
    assert enabled["health"] == "failing"
    assert enabled["healthError"]
    # The container starts before enable first evaluates it.
    assert waited >= window - 1
    assert "exasol sidecar status caddy" in guidance


def test_sidecar_enable_no_wait_reports_starting(service: SidecarDeployment) -> None:
    # Given
    _save_probe(service, {"exec": {"command": ["false"]}, "periodSeconds": 600})
    # When
    enabled = service.sidecar("enable", "--no-wait")
    guidance = _enable_text(service, "--no-wait")
    # Then
    assert enabled["health"] == "starting"
    assert "Sidecar caddy is still starting" in guidance
    assert "exasol sidecar status caddy" in guidance


def test_sidecar_internal_port(service: SidecarDeployment) -> None:
    # Given
    service.sidecar("enable")
    definition = service.definition()
    definition["ports"] = [{"containerPort": 8080}]
    # When
    service.save(definition)
    service.deployment.start()
    # Then
    assert service.sidecar("status")["endpoints"] == []
    assert service.internal_response().endswith("|initial-marker")


def test_sidecar_bidirectional_service_dns(service: SidecarDeployment) -> None:
    # Given
    service.sidecar("enable")
    service.response()
    # When
    service.database_reachable()
    response = service.podman(
        "run",
        "--rm",
        "--network",
        f"container:{service.database}",
        "--entrypoint",
        "wget",
        service.definition()["image"],
        "-qO-",
        "http://caddy:8080",
    )
    # Then
    assert response.endswith("|initial-marker")


def test_sidecar_existing_database_network(service: SidecarDeployment) -> None:
    # Given
    service.podman(
        "network",
        "connect",
        DEFAULT_NETWORKS[container_engine(service.directory)],
        service.database,
    )
    service.podman("network", "disconnect", service.network, service.database)
    original = service.inspect(service.database)["State"]["StartedAt"]
    assert "42" in service.sql()
    # When
    service.sidecar("enable")
    # Then
    assert service.inspect(service.database)["State"]["StartedAt"] == original
    service.database_reachable()
    assert "42" in service.sql()


def test_sidecar_loopback_endpoint(service: SidecarDeployment) -> None:
    # Given
    service.sidecar("enable")
    # When
    status = service.sidecar("status")
    response = service.response()
    # Then
    assert status["endpoints"] == [{"ip": "127.0.0.1", "port": 18080}]
    assert response.endswith("|initial-marker")
    with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as route:
        route.connect(("192.0.2.1", 9))
        host_address = route.getsockname()[0]
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as other_interface:
        other_interface.bind((host_address, 18080))


def test_sidecar_deployment_dns_scope(
    deployment_manager: DeploymentManager, infra: str
) -> None:
    # Given
    with concurrent_sidecar_deployments(deployment_manager, infra, 2) as (
        first,
        second,
    ):
        for service, marker in ((first, "first"), (second, "second")):
            service.deployment.launcher.run_command(
                "config", str(service.directory), "set", "--ports", "auto"
            )
            service.sidecar("enable")
            definition = service.definition()
            definition["ports"] = [{"containerPort": 8080}]
            next(v for v in definition["env"] if v["name"] == "TEST_MARKER")[
                "value"
            ] = marker
            service.save(definition)
            service.deployment.deploy()
        # When
        responses = [service.internal_response() for service in (first, second)]
        # Then
        assert responses[0].endswith("|first")
        assert responses[1].endswith("|second")
        for service in (first, second):
            service.database_reachable()
            database_ip = service.inspect(service.database)["NetworkSettings"][
                "Networks"
            ][service.network]["IPAddress"]
            lookup = service.podman(
                "exec", service.container, "nslookup", "-type=A", "database."
            )
            assert database_ip in lookup


def test_sidecar_database_without_alias_requires_restart(
    service: SidecarDeployment,
) -> None:
    # Given a database on the service network that does not answer to its alias
    service.podman("network", "disconnect", service.network, service.database)
    service.podman("network", "connect", service.network, service.database)
    original = service.inspect(service.database)["State"]["StartedAt"]
    # When
    pending = service.sidecar("enable")
    text = service.deployment.launcher.run_command(
        "sidecar",
        str(service.directory),
        "enable",
        "caddy",
        capture_output=True,
    )
    # Then
    assert pending["enabled"]
    assert not pending["running"]
    assert pending["restartRequired"]
    assert "exasol stop" in text.stderr
    assert "exasol start" in text.stderr
    assert service.inspect(service.database)["State"]["StartedAt"] == original
    assert "42" in service.sql()
    # When
    service.deployment.stop()
    service.deployment.start()
    # Then
    assert not service.sidecar("status")["restartRequired"]
    assert service.response().endswith("|initial-marker")


def test_sidecar_destroy_cleanup(service: SidecarDeployment) -> None:
    # Given
    service.sidecar("enable")
    service.response()
    # Destroy removes the engine record, so read it first.
    engine = shutil.which(container_engine(service.directory))
    # When
    service.deployment.destroy("--auto-approve")
    # Then
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as listener:
        listener.bind(("127.0.0.1", 18080))
    if sys.platform != "darwin":
        assert engine is not None
        for arguments in (
            [
                "ps",
                "-a",
                "--filter",
                f"name={service.container}",
                "--format",
                "{{.Names}}",
            ],
            [
                "network",
                "ls",
                "--filter",
                f"name={service.network}",
                "--format",
                "{{.Name}}",
            ],
        ):
            result = subprocess.run(
                [engine, *arguments],
                check=True,
                capture_output=True,
                text=True,
                timeout=30,
            )
            assert result.stdout.strip() == ""
    assert service.deployment.has_no_deployment()


@pytest.mark.chaos
@pytest.mark.parametrize("interruption", ["before-create", "after-create"])
def test_sidecar_interrupted_enable_recovery(
    service: SidecarDeployment,
    interruption: str,
) -> None:
    # Given
    service.sidecar("enable")
    saved = (service.directory / "sidecars.yaml").read_bytes()
    if interruption == "before-create":
        service.podman("rm", "--force", service.container)
    else:
        service.podman("stop", service.container)
    # When
    service.deployment.start()
    # Then
    assert (service.directory / "sidecars.yaml").read_bytes() == saved
    assert service.sidecar("status")["running"]
    assert service.response().endswith("|initial-marker")
    assert "42" in service.sql()
