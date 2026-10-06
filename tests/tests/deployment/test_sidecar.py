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

import pytest

from framework.deployment import StatusDatabaseReady
from framework.sidecars import SidecarDeployment, sidecar_deployment

pytestmark = pytest.mark.local_e2e


@pytest.fixture
def service(exasol_path: str, infra: str) -> Iterator[SidecarDeployment]:
    with sidecar_deployment(exasol_path, infra) as deployment:
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


def test_sidecar_password_opt_out(service: SidecarDeployment) -> None:
    # Given
    service.sidecar("enable")
    # When
    service.sidecar("enable", "--no-db-password")
    service.deployment.stop()
    service.deployment.start()
    service.sidecar("enable")
    # Then
    environment = service.podman("exec", service.container, "env").splitlines()
    assert not any(value.startswith("DB_PASSWORD=") for value in environment)
    assert "DB_HOST=database" in environment
    assert all(item["name"] != "DB_PASSWORD" for item in service.definition()["env"])


def test_sidecar_edited_password_opt_out(service: SidecarDeployment) -> None:
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


def test_sidecar_enable_idempotent(service: SidecarDeployment) -> None:
    # Given
    service.sidecar("enable")
    original = service.inspect()["Id"]
    assert service.response().endswith("|initial-marker")
    # When
    service.sidecar("enable")
    # Then
    assert service.inspect()["Id"] == original
    assert service.sidecar("status")["hosts"][0]["running"]
    assert service.response().endswith("|initial-marker")


@pytest.mark.parametrize("initial_state", ["initialized", "stopped"])
def test_sidecar_deferred_enable(
    exasol_path: str,
    infra: str,
    initial_state: str,
) -> None:
    # Given
    with sidecar_deployment(exasol_path, infra, running=False) as service:
        if initial_state == "stopped":
            service.deployment.deploy()
            service.deployment.stop()
        # When
        status = service.sidecar("enable")
        # Then
        assert status["enabled"]
        assert not status["hosts"][0]["running"]
        # When
        if initial_state == "initialized":
            service.deployment.deploy()
        else:
            service.deployment.start()
        # Then
        assert service.sidecar("status")["hosts"][0]["running"]
        assert service.response().endswith("|initial-marker")


def test_sidecar_start_reconciles_marker(service: SidecarDeployment) -> None:
    # Given
    service.sidecar("enable")
    original = service.inspect()["Id"]
    definition = service.definition()
    next(v for v in definition["env"] if v["name"] == "TEST_MARKER")["value"] = "edited"
    # When
    service.save(definition)
    service.deployment.start()
    # Then
    assert service.response().endswith("|edited")
    assert service.inspect()["Id"] != original
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
    recorded = service.directory / "sidecars-state.json"
    failure = {"local/caddy": "previous publication failure"}
    recorded.write_text(json.dumps(failure), encoding="utf-8")
    # When
    current = service.sidecar("status")["hosts"][0]
    # Then
    assert current["reconciliation"] == "complete"
    assert "lastOperationError" not in current
    assert json.loads(recorded.read_text(encoding="utf-8")) == {}
    assert service.inspect()["Id"] == original
    # When
    edited = service.definition()
    next(v for v in edited["env"] if v["name"] == "TEST_MARKER")["value"] = "pending"
    service.save(edited)
    recorded.write_text(json.dumps(failure), encoding="utf-8")
    pending = service.sidecar("status")["hosts"][0]
    # Then
    assert pending["running"]
    assert pending["reconciliation"] == "pending"
    assert pending["lastOperationError"] == failure["local/caddy"]
    assert service.inspect()["Id"] == original
    assert service.response().endswith("|initial-marker")
    # When
    service.save(definition)
    restored = service.sidecar("status")["hosts"][0]
    # Then
    assert restored["reconciliation"] == "complete"
    assert "lastOperationError" not in restored
    assert service.inspect()["Id"] == original
    # When
    service.sidecar("disable")
    recorded.write_text(json.dumps(failure), encoding="utf-8")
    removed = service.sidecar("status")["hosts"][0]
    # Then
    assert removed["reconciliation"] == "complete"
    assert "lastOperationError" not in removed
    assert json.loads(recorded.read_text(encoding="utf-8")) == {}


def test_sidecar_start_removes_deleted_definition(service: SidecarDeployment) -> None:
    # Given
    service.sidecar("enable")
    # When
    service.save()
    service.deployment.start()
    # Then
    assert not service.sidecar("status")["enabled"]
    assert (
        json.loads(
            service.podman(
                "ps", "-a", "--filter", f"name={service.container}", "--format", "json"
            )
        )
        == []
    )
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as listener:
        listener.bind(("127.0.0.1", 18080))


def test_sidecar_deployment_restart(service: SidecarDeployment) -> None:
    # Given
    service.sidecar("enable")
    original = service.inspect()["Id"]
    # When
    service.deployment.stop()
    stopped = service.sidecar("status")
    service.deployment.start()
    # Then
    assert stopped["enabled"]
    assert not stopped["hosts"][0]["running"]
    assert service.inspect()["Id"] != original
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
    assert first == second
    assert not second["enabled"]
    assert not second["hosts"][0]["running"]
    assert service.inspect(service.database)["Id"] == database
    assert "42" in service.sql("SELECT N FROM SIDECAR_TEST.T;")


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
    assert running["hosts"][0]["running"]
    assert "caddy: enabled" in text
    assert "Host local: running" in text
    assert running["hosts"][0]["endpoints"] == [{"ip": "127.0.0.1", "port": 18080}]
    assert response.endswith("|initial-marker")
    assert "127.0.0.1:18080" in text
    assert stopped["enabled"]
    assert not stopped["hosts"][0]["running"]
    assert stopped["hosts"][0]["endpoints"] == []
    assert "caddy: enabled" in stopped_text
    assert "Host local: stopped" in stopped_text


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
    assert service.sidecar("status")["hosts"][0]["lastOperationError"]
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


def test_sidecar_internal_port(service: SidecarDeployment) -> None:
    # Given
    service.sidecar("enable")
    definition = service.definition()
    definition["ports"] = [{"containerPort": 8080}]
    # When
    service.save(definition)
    service.deployment.start()
    # Then
    assert service.sidecar("status")["hosts"][0]["endpoints"] == []
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
    service.podman("network", "connect", "podman", service.database)
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
    assert status["hosts"][0]["endpoints"] == [{"ip": "127.0.0.1", "port": 18080}]
    assert response.endswith("|initial-marker")
    with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as route:
        route.connect(("192.0.2.1", 9))
        host_address = route.getsockname()[0]
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as other_interface:
        other_interface.bind((host_address, 18080))


def test_sidecar_deployment_dns_scope(exasol_path: str, infra: str) -> None:
    # Given
    with (
        sidecar_deployment(exasol_path, infra, running=False) as first,
        sidecar_deployment(exasol_path, infra, running=False) as second,
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


@pytest.mark.skipif(
    sys.platform != "linux", reason="legacy rootless networking uses host Podman"
)
def test_sidecar_legacy_network_restart(service: SidecarDeployment) -> None:
    # Given
    command = list(service.inspect(service.database)["Config"]["CreateCommand"])
    for option in ("--network", "--network-alias"):
        index = command.index(option)
        del command[index : index + 2]
    pasta = service.host("sh", "-c", "command -v pasta || true").strip()
    command[2:2] = ["--network", "pasta" if pasta else "slirp4netns"]
    service.podman("stop", service.database)
    service.podman("rm", service.database)
    service.host(*command)
    deadline = time.monotonic() + 120
    while True:
        try:
            assert "42" in service.sql()
            break
        except CalledProcessError:
            if time.monotonic() >= deadline:
                raise
            time.sleep(1)
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
    assert not pending["hosts"][0]["running"]
    assert pending["hosts"][0]["restartRequired"]
    assert "exasol stop" in text.stderr
    assert "exasol start" in text.stderr
    assert service.inspect(service.database)["State"]["StartedAt"] == original
    assert "42" in service.sql()
    # When
    service.deployment.stop()
    service.deployment.start()
    # Then
    assert not service.sidecar("status")["hosts"][0]["restartRequired"]
    assert service.response().endswith("|initial-marker")


def test_sidecar_destroy_cleanup(service: SidecarDeployment) -> None:
    # Given
    service.sidecar("enable")
    service.response()
    # When
    service.deployment.destroy("--auto-approve")
    # Then
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as listener:
        listener.bind(("127.0.0.1", 18080))
    if sys.platform != "darwin":
        podman = shutil.which("podman")
        assert podman is not None
        for arguments in (
            ["ps", "-a", "--filter", f"name={service.container}", "--format", "json"],
            [
                "network",
                "ls",
                "--filter",
                f"name={service.network}",
                "--format",
                "json",
            ],
        ):
            result = subprocess.run(
                [podman, *arguments],
                check=True,
                capture_output=True,
                text=True,
                timeout=30,
            )
            assert json.loads(result.stdout) == []
    assert service.deployment.has_no_deployment()
