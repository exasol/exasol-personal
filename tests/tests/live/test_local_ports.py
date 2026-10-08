# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

"""Tests specific to local deployments."""

import json
import socket
from collections.abc import Iterator
from ipaddress import ip_address
from pathlib import Path
from subprocess import CalledProcessError
from typing import Final, NamedTuple, cast

import pytest

from framework.deployment import Deployment, DeploymentManager, StatusStopped
from framework.launcher import DeploymentConfig

pytestmark = [
    pytest.mark.openspec("exasol-local-deployment"),
    pytest.mark.providers("local"),
    pytest.mark.platform("linux-amd64", "macos-arm64", "windows-amd64"),
]

AUTOMATIC_BLOCKED_PORTS: Final = (8563, 8564)
NON_LOOPBACK_CONNECT_TIMEOUT_SECONDS: Final = 3
# Documentation-only address (RFC 5737); connecting a UDP socket sends nothing.
ROUTE_PROBE_ADDRESS: Final = "192.0.2.1"
EXPECTED_AUTOMATIC_PORT: Final = 8565
EPHEMERAL_RESERVATION_ATTEMPTS: Final = 20


class _LoopbackAddress(NamedTuple):
    family: socket.AddressFamily
    host: str
    flowinfo: int
    scope_id: int


class _NoLoopbackAddressesError(OSError):
    pass


class _EphemeralReservationExhaustedError(OSError):
    pass


def _configured_db_port(deployment: Deployment) -> int:
    result = deployment.launcher.run_command(
        "config",
        deployment.deployment_dir.name,
        "get",
        "ports",
        "--json",
        capture_output=True,
    )
    data = json.loads(result.stdout)
    mapping = data["infrastructure"]["options"]["ports"]
    service, separator, raw_port = mapping.partition(":")
    assert (service, separator) == ("db", ":")

    return int(raw_port)


def _reported_db_port(deployment: Deployment) -> int:
    deployment_json = Path(deployment.deployment_dir.name) / "deployment.json"

    return int(json.loads(deployment_json.read_text())["connection"]["dbPort"])


def _localhost_loopback_addresses() -> list[_LoopbackAddress]:
    addresses: list[_LoopbackAddress] = []
    seen: set[tuple[socket.AddressFamily, str, int]] = set()
    for family, _, _, _, sockaddr in socket.getaddrinfo(
        "localhost",
        0,
        type=socket.SOCK_STREAM,
        proto=socket.IPPROTO_TCP,
    ):
        if family not in (socket.AF_INET, socket.AF_INET6):
            continue
        address_parts = cast("tuple[object, ...]", sockaddr)
        host = str(address_parts[0])
        normalized_host = str(ip_address(host.split("%", maxsplit=1)[0]))
        if not ip_address(normalized_host).is_loopback:
            continue
        flowinfo = cast("int", address_parts[2]) if family == socket.AF_INET6 else 0
        scope_id = cast("int", address_parts[3]) if family == socket.AF_INET6 else 0
        address_family = socket.AddressFamily(family)
        key = (address_family, normalized_host, scope_id)
        if key in seen:
            continue
        seen.add(key)
        addresses.append(
            _LoopbackAddress(address_family, normalized_host, flowinfo, scope_id)
        )
    if not addresses:
        raise _NoLoopbackAddressesError

    return addresses


def _non_loopback_ipv4_addresses() -> list[str]:
    candidates: set[str] = set()
    try:
        for *_, sockaddr in socket.getaddrinfo(
            socket.gethostname(), None, socket.AF_INET
        ):
            candidates.add(str(sockaddr[0]))
    except OSError:
        pass
    probe = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    try:
        probe.connect((ROUTE_PROBE_ADDRESS, 9))
        candidates.add(str(probe.getsockname()[0]))
    except OSError:
        pass
    finally:
        probe.close()

    return sorted(
        address
        for address in candidates
        if not ip_address(address).is_loopback
        and not ip_address(address).is_unspecified
    )


def _accepts_connection(host: str, port: int) -> bool:
    try:
        with socket.create_connection(
            (host, port), timeout=NON_LOOPBACK_CONNECT_TIMEOUT_SECONDS
        ):
            return True
    except OSError:
        return False


def _diag_local(deployment: Deployment) -> dict[str, object]:
    result = deployment.launcher.run_command(
        "diag",
        deployment.deployment_dir.name,
        "local",
        capture_output=True,
    )
    diagnostics: dict[str, object] = json.loads(result.stdout)

    return diagnostics


def _close_listeners(listeners: list[socket.socket]) -> None:
    for listener in listeners:
        listener.close()


def _listen_on_loopback(address: _LoopbackAddress, port: int) -> socket.socket:
    listener = socket.socket(address.family, socket.SOCK_STREAM, socket.IPPROTO_TCP)
    try:
        bind_address: tuple[str, int] | tuple[str, int, int, int]
        if address.family == socket.AF_INET6:
            listener.setsockopt(socket.IPPROTO_IPV6, socket.IPV6_V6ONLY, 1)
            bind_address = (
                address.host,
                port,
                address.flowinfo,
                address.scope_id,
            )
        else:
            bind_address = (address.host, port)
        listener.bind(bind_address)
        listener.listen()
    except OSError:
        listener.close()
        raise

    return listener


def _reserve_port(port: int = 0) -> tuple[list[socket.socket], int]:
    addresses = _localhost_loopback_addresses()
    attempts = EPHEMERAL_RESERVATION_ATTEMPTS if port == 0 else 1
    for _ in range(attempts):
        listeners: list[socket.socket] = []
        selected_port = port
        try:
            for address in addresses:
                listener = _listen_on_loopback(address, selected_port)
                if selected_port == 0:
                    selected_port = int(listener.getsockname()[1])
                listeners.append(listener)

        except OSError:
            _close_listeners(listeners)
            if port != 0:
                raise
        else:
            return listeners, selected_port

    raise _EphemeralReservationExhaustedError


def _verify_occupied_port_recovery(deployment: Deployment) -> None:
    # Given a stopped deployment is reconfigured to an occupied port
    deployment.stop()
    conflict, conflict_port = _reserve_port()
    try:
        set_result = deployment.launcher.run_command(
            "config",
            deployment.deployment_dir.name,
            "set",
            "--ports",
            f"db:{conflict_port}",
            capture_output=True,
        )
        assert "exasol start" in set_result.stderr

        # When start reaches the authoritative runtime bind
        with pytest.raises(CalledProcessError) as captured:
            deployment.launcher.run_command(
                "start",
                deployment.deployment_dir.name,
                "--auto-approve",
                capture_output=True,
            )

        # Then the terminal offers actionable recovery, whether or not the
        # launcher could identify the cause on this platform
        assert "exasol config set --ports db:<available-port>" in captured.value.stderr
        assert "exasol config set --ports auto" in captured.value.stderr
        assert _configured_db_port(deployment) == conflict_port
    finally:
        _close_listeners(conflict)

    # When automatic replacement is selected after releasing the conflict.
    # This succeeding is also what proves the failed start left the deployment
    # reconfigurable: `config set` is rejected in every other post-deploy state.
    reset_result = deployment.launcher.run_command(
        "config",
        deployment.deployment_dir.name,
        "set",
        "--ports",
        "auto",
        capture_output=True,
    )
    replacement_port = _configured_db_port(deployment)
    assert replacement_port > 0
    assert "exasol start" in reset_result.stderr
    deployment.start()

    # Then the replacement endpoint is persisted and reachable
    assert _reported_db_port(deployment) == replacement_port
    proc = deployment.connect(input="SELECT * FROM Dual", capture_output=True)
    assert "DUMMY" in proc.stdout


@pytest.fixture
def local_ports_deployment(
    deployment_manager: DeploymentManager,
) -> Iterator[tuple[Deployment, int]]:
    custom_db_port: Final = 9564
    config = DeploymentConfig(infra="local")

    deployment = deployment_manager.local(
        "--ports",
        f"db:{custom_db_port}",
        config=config,
    )
    try:
        yield deployment, custom_db_port
    finally:
        deployment_manager.release(deployment)


def test_ports_override_sets_db_port(
    local_ports_deployment: tuple[Deployment, int],
) -> None:
    """--ports db:<port> passes the port to the selected local runtime.

    The DB is reachable on the specified port on loopback only.
    """
    deployment, custom_db_port = local_ports_deployment

    deployment_json = Path(deployment.deployment_dir.name) / "deployment.json"
    info = json.loads(deployment_json.read_text())
    assert info["connection"]["dbPort"] == custom_db_port

    proc = deployment.connect(input="SELECT * FROM Dual", capture_output=True)
    assert "DUMMY" in proc.stdout

    assert _accepts_connection("127.0.0.1", custom_db_port)
    for address in _non_loopback_ipv4_addresses():
        assert not _accepts_connection(address, custom_db_port), address


def test_ports_override_stable_across_restarts(
    local_ports_deployment: tuple[Deployment, int],
) -> None:
    """Port assignments from --ports db:<port> survive a stop/start cycle.

    The custom DB port must remain unchanged in deployment.json and the DB
    must be reachable on that port after the local runtime is restarted.
    """
    deployment, custom_db_port = local_ports_deployment

    deployment_json = Path(deployment.deployment_dir.name) / "deployment.json"

    stop_result = deployment.stop()
    assert stop_result.returncode == 0

    info = json.loads(deployment_json.read_text())
    assert info["connection"]["dbPort"] == custom_db_port

    start_result = deployment.start()
    assert start_result.returncode == 0

    info = json.loads(deployment_json.read_text())
    assert info["connection"]["dbPort"] == custom_db_port

    proc = deployment.connect(input="SELECT * FROM Dual", capture_output=True)
    assert "DUMMY" in proc.stdout


def test_static_local_port_selection_reconfiguration_and_recovery(
    deployment_manager: DeploymentManager,
) -> None:
    """Automatic ports are deterministic, stable, configurable, and recoverable."""
    # Given the database default and its successor are occupied during init
    reservations: list[socket.socket] = []
    deployment: Deployment | None = None
    try:
        try:
            for port in AUTOMATIC_BLOCKED_PORTS:
                listeners, _ = _reserve_port(port)
                reservations.extend(listeners)
            probe, _ = _reserve_port(EXPECTED_AUTOMATIC_PORT)
            _close_listeners(probe)
        except OSError as exc:
            _close_listeners(reservations)
            pytest.skip(f"required deterministic test ports are unavailable: {exc}")

        # When a local deployment is initialized with automatic ports
        deployment = deployment_manager.local(
            deploy=False,
            config=DeploymentConfig(infra="local"),
        )

        # Then allocation advances deterministically and persists the concrete value
        assert _configured_db_port(deployment) == EXPECTED_AUTOMATIC_PORT
        _close_listeners(reservations)
        reservations.clear()

        # When the deployment is started, stopped, and started again
        deployment.deploy()
        assert _reported_db_port(deployment) == EXPECTED_AUTOMATIC_PORT
        deployment.stop()
        assert deployment.status_value() == StatusStopped
        deployment.start()

        # Then its runtime endpoint remains stable
        assert _configured_db_port(deployment) == EXPECTED_AUTOMATIC_PORT
        assert _reported_db_port(deployment) == EXPECTED_AUTOMATIC_PORT

        _verify_occupied_port_recovery(deployment)
    finally:
        _close_listeners(reservations)
        if deployment is not None:
            deployment_manager.release(deployment)


@pytest.mark.openspec("local-reachability-diagnostics")
def test_diag_local_reports_runtime_state_when_running_and_stopped(
    local_ports_deployment: tuple[Deployment, int],
) -> None:
    """`diag local` succeeds in both states and reports what each state allows."""
    deployment, db_port = local_ports_deployment

    # When diagnostics are collected for the running deployment
    running = _diag_local(deployment)

    # Then the runtime, its bound port, reachability, and readiness are reported
    assert running["platformSupported"] is True
    assert running["vmRunning"] is True
    assert running["ports"] == {"db": db_port}
    assert running["portHealth"] == {"db": "reachable"}
    assert running["databaseReady"] is True

    # When diagnostics are collected after the deployment is stopped
    deployment.stop()
    stopped = _diag_local(deployment)

    # Then the stopped runtime is reported with guidance to start it
    assert stopped["platformSupported"] is True
    assert stopped["vmRunning"] is False
    assert "exasol start" in str(stopped["message"])
    assert "databaseReady" not in stopped
