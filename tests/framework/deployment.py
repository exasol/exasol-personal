# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

import json
import logging
import tempfile
import time
from pathlib import Path
from subprocess import CompletedProcess, Popen
from typing import Final, Unpack

import websocket

from framework.launcher import DeploymentConfig, Launcher
from framework.outputs import get_outputs
from framework.types import SubprocessRunKwargs

StatusInitialized = "initialized"
StatusOperationInProgress = "operation_in_progress"
StatusInterrupted = "interrupted"
StatusDeploymentFailed = "deployment_failed"
StatusDatabaseConnectionFailed = "database_connection_failed"
StatusDatabaseReady = "database_ready"
StatusStopped = "stopped"


class Deployment:
    DEPLOYMENT_DIR_PREFIX: Final = "exasol-launcher-"

    def __init__(
        self,
        launcher: Launcher,
        *args: str,
        config: DeploymentConfig,
    ) -> None:
        """Initialize a new deployment.

        Args:
            launcher: The launcher to use for managing the deployment.
            args: Additional arguments to `launcher init`
            config: Configuration for the deployment including cluster size,
                instance type, volume sizes, and passwords.

        """
        self.launcher = launcher

        self.deployment_dir = tempfile.TemporaryDirectory(
            prefix=self.DEPLOYMENT_DIR_PREFIX,
        )
        logging.info(
            "Created deployment directory for test: %s",
            self.deployment_dir.name,
        )

        self.launcher.init(
            self.deployment_dir.name,
            *args,
            config=config,
        )

        if not self.launcher.has_status(self.deployment_dir.name, StatusInitialized):
            msg = f"Expected status `{StatusInitialized}` after `init`"
            raise RuntimeError(msg)

        if not self.has_no_deployment():
            msg = """no file should exist and 'initialized but not yet completed'
                    msg after `init`"""
            raise RuntimeError(msg)

    def cleanup(self) -> None:
        """Clean up the active deployment and its deployment directory.

        This will destroy all cloud resources if they exist and remove the
        deployment directory. Safe to call even if resources were already
        destroyed or were never deployed.
        """
        logging.info(
            "Destroying deployment and cleaning up directory: %s",
            self.deployment_dir.name,
        )

        self.launcher.destroy(self.deployment_dir.name, "--auto-approve")

        if not self.launcher.has_status(self.deployment_dir.name, StatusInitialized):
            msg = f"Expected status `{StatusInitialized}` after `destroy`"
            raise RuntimeError(msg)

        logging.info("Verifying deployment info after destroy")

        if not self.has_no_deployment():
            msg = """no file should exist and 'initialized but not yet completed'
                    msg after `init`"""
            raise RuntimeError(msg)

        self.deployment_dir.cleanup()

    def deploy(self, *args: str) -> CompletedProcess[str]:
        return self.launcher.deploy(self.deployment_dir.name, *args)

    def deploy_no_block(self, *args: str) -> Popen[str]:
        return self.launcher.deploy_no_block(self.deployment_dir.name, *args)

    def start_no_block(self, *args: str) -> Popen[str]:
        return self.launcher.start_no_block(self.deployment_dir.name, *args)

    def stop_no_block(self, *args: str) -> Popen[str]:
        return self.launcher.stop_no_block(self.deployment_dir.name, *args)

    def destroy(self, *args: str) -> CompletedProcess[str]:
        return self.launcher.destroy(self.deployment_dir.name, *args)

    def start(self, *args: str) -> CompletedProcess[str]:
        """Start the deployment (power on)."""
        return self.launcher.start(self.deployment_dir.name, *args)

    def stop(self, *args: str) -> CompletedProcess[str]:
        """Stop the deployment (power off)."""
        return self.launcher.stop(self.deployment_dir.name, *args)

    def info(self, *args: str) -> CompletedProcess[str]:
        """Deployment Information."""
        return self.launcher.info(self.deployment_dir.name, *args)

    def connect(
        self,
        *args: str,
        **kwargs: Unpack[SubprocessRunKwargs],
    ) -> CompletedProcess[str]:
        return self.launcher.connect(
            self.deployment_dir.name,
            *args,
            **kwargs,
        )

    def status(self, *args: str) -> CompletedProcess[str]:
        return self.launcher.status(self.deployment_dir.name, *args)

    def has_status(self, expected_status: str) -> bool:
        return self.launcher.has_status(
            self.deployment_dir.name,
            expected_status=expected_status,
        )

    def has_no_deployment(self) -> bool:
        return self.launcher.has_no_deployment(
            self.deployment_dir.name,
        )

    def wait_stopped(self, timeout: int = 180, interval: int = 5) -> None:
        """Wait until the deployment is stopped (DB not connectable)."""
        start_time = time.time()

        while time.time() - start_time < timeout:
            try:
                if not self.db_connectable():
                    return
            except (websocket.WebSocketException, OSError, json.JSONDecodeError) as exc:
                # Connection problems or errors indicate DB is down/stopped
                logging.info("DB not connectable (exception): %s", exc)
                return

            time.sleep(interval)

        message = "Deployment did not stop within timeout"
        raise TimeoutError(message)

    def _get_public_ip(self, node_id: int) -> str:
        logging.info("Getting public IP of a node: %s", node_id)

        outputs = get_outputs(self.deployment_dir.name)

        return outputs.nodes[f"n{node_id}"].publicIp

    def admin_ui(self, node_id: int = 11) -> tuple[str, str]:
        """Get the admin UI hostname and port for a given node.

        Args:
            node_id: The node ID (defaults to 11, typically first data node).

        Returns:
            A tuple of (hostname, port) with public IP and UI port.

        """
        logging.info("Getting admin UI host and port for node: %s", node_id)

        outputs = get_outputs(self.deployment_dir.name)
        node = outputs.nodes[f"n{node_id}"]

        return node.publicIp, node.database.uiPort

    def admin_ui_credentials(self) -> tuple[str, str]:
        """Get the admin UI username and password from the secrets file.

        Returns:
            A tuple of (username, password) where username is always "admin".

        """
        logging.info("Getting admin UI credentials from secrets file")

        secrets_glob = "secrets-exasol-*.json"
        deployment_dir_path = Path(self.deployment_dir.name)

        # Find the secrets file
        secrets_files = list(deployment_dir_path.glob(secrets_glob))
        if not secrets_files:
            msg = (
                f"No secrets file matching {secrets_glob} found in "
                f"{self.deployment_dir.name}"
            )
            raise FileNotFoundError(msg)

        secrets_file = secrets_files[0]
        logging.info("Reading secrets from: %s", secrets_file)

        with secrets_file.open() as f:
            secrets_data = json.load(f)

        admin_password = secrets_data.get("adminUiPassword")
        if not admin_password:
            msg = "adminUiPassword not found in secrets file"
            raise ValueError(msg)

        return "admin", admin_password

    def deployment_id(self) -> str:
        """Get the deployment ID from the outputs file.

        Returns:
            The deployment ID as a string.

        """
        logging.info("Getting deployment ID")

        outputs = get_outputs(self.deployment_dir.name)
        return outputs.deploymentId

    def db_connectable(self) -> bool:
        """Check and return true if the DB is connectable over WebSocket."""
        logging.info("Checking if the database is connectable")

        db_port: Final = 8563

        # For now we assume that if n11 is connectable then all
        # the nodes are connectable. Subject to change.
        n11_ip = self._get_public_ip(node_id=11)

        # Skipping certificate verification.
        conn = websocket.WebSocket(sslopt={"cert_reqs": 0})

        try:
            conn.connect(f"wss://{n11_ip}:{db_port}")  # type: ignore[no-untyped-call]
            conn.send('{"command":"login","protocolVersion":2}')
            response = conn.recv()
        finally:
            conn.close()

        status: str = json.loads(response)["status"]

        return status == "ok"
