# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

import logging
import tempfile
from pathlib import Path
from subprocess import TimeoutExpired
from typing import TYPE_CHECKING, cast

import pytest

from framework.deployment import Deployment
from framework.launcher import DeploymentConfig

if TYPE_CHECKING:
    from framework.launcher import Launcher


class _TimeoutLauncher:
    def __init__(self, command: str) -> None:
        self.command = command
        self.calls: list[tuple[tuple[str, ...], dict[str, object]]] = []

    def deploy(self, *args: str, **kwargs: object) -> None:
        self.calls.append((args, kwargs))
        raise TimeoutExpired(self.command, 1200)

    def destroy(self, *args: str, **kwargs: object) -> None:
        self.calls.append((args, kwargs))
        raise TimeoutExpired(self.command, 1200)


class _WebSocket:
    def __init__(self, sslopt: dict[str, int]) -> None:
        self.sslopt = sslopt
        self.url = ""

    def connect(self, url: str) -> None:
        self.url = url

    def send(self, message: str) -> None:
        assert message == '{"command":"login","protocolVersion":2}'

    def recv(self) -> str:
        return '{"status":"ok"}'

    def close(self) -> None:
        pass


def test_db_connectable_uses_normalized_local_connection(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    # Given
    deployment = object.__new__(Deployment)
    deployment.deployment_dir = tempfile.TemporaryDirectory(dir=tmp_path)
    Path(deployment.deployment_dir.name, "deployment.json").write_text(
        '{"deploymentId":"local","connection":{"host":"127.0.0.1","dbPort":28563}}',
        encoding="utf-8",
    )
    websocket = _WebSocket({"cert_reqs": 0})
    monkeypatch.setattr(
        "framework.deployment.websocket.WebSocket", lambda **_: websocket
    )

    # When
    connectable = deployment.db_connectable()

    # Then
    assert connectable
    assert websocket.url == "wss://127.0.0.1:28563"


def test_deploy_timeout_reports_deployment_log_tail(tmp_path: Path) -> None:
    # Given: a deployment command that exceeds the shared test timeout.
    deployment = object.__new__(Deployment)
    deployment.deployment_dir = tempfile.TemporaryDirectory(dir=tmp_path)
    Path(deployment.deployment_dir.name, "deployment.log").write_text(
        "last deployment progress\n",
        encoding="utf-8",
    )
    launcher = _TimeoutLauncher("deploy")
    deployment.launcher = cast("Launcher", launcher)

    # When: the test framework deploys the cluster.
    with pytest.raises(TimeoutError, match="last deployment progress"):
        deployment.deploy()

    # Then: it bounds the command and retains useful cloud-install diagnostics.
    assert launcher.calls == [
        (
            (deployment.deployment_dir.name, "--auto-approve"),
            {"timeout": Deployment.DEPLOY_TIMEOUT_SECONDS},
        )
    ]


def test_cleanup_preserves_deployment_log_when_destroy_times_out(
    caplog: pytest.LogCaptureFixture,
) -> None:
    # Given: cleanup cannot finish its destroy command within the test timeout.
    launcher = _TimeoutLauncher("destroy")
    deployment = Deployment(
        cast("Launcher", launcher),
        config=DeploymentConfig(infra="aws"),
        initialize=False,
    )
    Path(deployment.deployment_dir.name, "deployment.log").write_text(
        "last deployment progress\n",
        encoding="utf-8",
    )

    # When: the framework cleans up the failed deployment.
    with (
        caplog.at_level(logging.ERROR),
        pytest.raises(RuntimeError, match="Failed to clean up deployment"),
    ):
        deployment.cleanup()

    # Then: its diagnostics remain available for the test failure report.
    assert Path(deployment.deployment_dir.name).exists()
    assert "last deployment progress" in caplog.text
    deployment.deployment_dir.cleanup()
