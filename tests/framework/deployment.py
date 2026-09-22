# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

import json
import logging
import secrets
import tempfile
import time
from dataclasses import replace
from pathlib import Path
from subprocess import CalledProcessError, CompletedProcess, Popen, TimeoutExpired
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
    AZURE_NIC_RESERVATION_ERROR: Final = "NicReservedForAnotherVm"
    DESTROY_RETRY_MAX_ATTEMPTS: Final = 5
    DESTROY_RETRY_INITIAL_DELAY_SECONDS: Final = 15.0
    DESTROY_RETRY_MAX_DELAY_SECONDS: Final = 200.0
    DESTROY_RETRY_JITTER_FACTOR: Final = 0.20
    DESTROY_TIMEOUT_SECONDS: Final = 20 * 60
    DEPLOY_TIMEOUT_SECONDS: Final = 20 * 60
    UNATTENDED_FLAG: Final = "--auto-approve"

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
        destroyed or were never deployed. Raises RuntimeError when destruction
        cannot be verified so callers cannot silently leave billable resources.
        """
        logging.info(
            "Destroying deployment and cleaning up directory: %s",
            self.deployment_dir.name,
        )

        cleanup_error = self._destroy_with_retries()

        if cleanup_error is not None:
            logging.error(
                "Cleanup did not complete for deployment_dir=%s: %s\n"
                "deployment.log tail:\n%s\n"
                "Deployment directory is preserved for manual investigation/cleanup.",
                self.deployment_dir.name,
                cleanup_error,
                self.deployment_log_tail(),
            )
            message = (
                f"Failed to clean up deployment {self.deployment_dir.name}: "
                f"{cleanup_error}"
            )
            raise RuntimeError(message)

        self.deployment_dir.cleanup()

    def _destroy_with_retries(self) -> str | None:
        """Destroy the deployment with exponential backoff for retryable errors."""
        delay_seconds = self.DESTROY_RETRY_INITIAL_DELAY_SECONDS

        for attempt in range(1, self.DESTROY_RETRY_MAX_ATTEMPTS + 1):
            attempt_error: str | None = None
            try:
                self.launcher.destroy(
                    self.deployment_dir.name,
                    "--auto-approve",
                    timeout=self.DESTROY_TIMEOUT_SECONDS,
                )

                if not self.launcher.has_status(
                    self.deployment_dir.name,
                    StatusInitialized,
                ):
                    attempt_error = (
                        f"Expected status `{StatusInitialized}` after `destroy`"
                    )

                logging.info("Verifying deployment info after destroy")

                if attempt_error is None and not self.has_no_deployment():
                    attempt_error = (
                        "no file should exist and 'initialized but not yet completed' "
                        "msg after `init`"
                    )
            except (CalledProcessError, RuntimeError, TimeoutExpired) as exc:
                attempt_error = str(exc)

            if attempt_error is None:
                return None

            log_tail = self.deployment_log_tail()
            retryable = self._is_retryable_destroy_error(log_tail)
            if (not retryable) or (attempt == self.DESTROY_RETRY_MAX_ATTEMPTS):
                return attempt_error

            jitter = delay_seconds * self.DESTROY_RETRY_JITTER_FACTOR
            jitter_fraction = secrets.randbelow(1001) / 1000.0
            wait_seconds = delay_seconds + (jitter * jitter_fraction)
            logging.warning(
                "Retryable destroy failure for deployment_dir=%s "
                "(attempt %s/%s). Waiting %.1fs before retry.\n"
                "deployment.log tail:\n%s",
                self.deployment_dir.name,
                attempt,
                self.DESTROY_RETRY_MAX_ATTEMPTS,
                wait_seconds,
                log_tail,
            )
            time.sleep(wait_seconds)
            delay_seconds = min(
                delay_seconds * 2.0,
                self.DESTROY_RETRY_MAX_DELAY_SECONDS,
            )

        return "Destroy retries exhausted"

    def _is_retryable_destroy_error(self, log_tail: str) -> bool:
        """Return true when the destroy failure is considered transient."""
        return self.AZURE_NIC_RESERVATION_ERROR in log_tail

    def _unattended(self, args: tuple[str, ...]) -> list[str]:
        """Add --auto-approve unless the caller already supplied it.

        Deployment tests run without a user. On platforms where the launcher
        must change the host to satisfy prerequisites — installing Podman on
        Windows — an unapproved deploy fails instead of proceeding. The flag is
        inert for backends that request no host changes.
        """
        if self.UNATTENDED_FLAG in args:
            return list(args)

        return [*args, self.UNATTENDED_FLAG]

    def deploy(
        self,
        *args: str,
        **kwargs: Unpack[SubprocessRunKwargs],
    ) -> CompletedProcess[str]:
        timeout = kwargs.get("timeout", self.DEPLOY_TIMEOUT_SECONDS)
        if "timeout" not in kwargs:
            kwargs["timeout"] = timeout
        try:
            return self.launcher.deploy(
                self.deployment_dir.name,
                *self._unattended(args),
                **kwargs,
            )
        except TimeoutExpired as exc:
            msg = (
                f"Deploy command timed out after {timeout}s\n"
                f"deployment.log tail:\n{self.deployment_log_tail()}"
            )
            raise TimeoutError(msg) from exc

    def deploy_no_block(self, *args: str) -> Popen[str]:
        return self.launcher.deploy_no_block(
            self.deployment_dir.name, *self._unattended(args)
        )

    def start_no_block(self, *args: str) -> Popen[str]:
        return self.launcher.start_no_block(
            self.deployment_dir.name, *self._unattended(args)
        )

    def stop_no_block(self, *args: str) -> Popen[str]:
        return self.launcher.stop_no_block(self.deployment_dir.name, *args)

    def destroy(self, *args: str) -> CompletedProcess[str]:
        return self.launcher.destroy(self.deployment_dir.name, *args)

    def start(self, *args: str) -> CompletedProcess[str]:
        """Start the deployment (power on)."""
        return self.launcher.start(self.deployment_dir.name, *self._unattended(args))

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

    def status_value(self) -> str:
        """Return the launcher-reported deployment status."""
        return self.launcher.status_value(self.deployment_dir.name)

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

        deployment_dir_path = Path(self.deployment_dir.name)

        secrets_file = deployment_dir_path / "secrets.json"
        if not secrets_file.exists():
            msg = (
                "No secrets file found: expected 'secrets.json' in "
                f"{self.deployment_dir.name}"
            )
            raise FileNotFoundError(msg)

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

    def deployment_log_tail(self, max_lines: int = 200) -> str:
        """Return the tail of deployment.log for diagnostics."""
        log_file = Path(self.deployment_dir.name) / "deployment.log"
        if not log_file.exists():
            return "<deployment.log not found>"

        with log_file.open(encoding="utf-8", errors="replace") as f:
            lines = f.readlines()

        return "".join(lines[-max_lines:]).strip()


class DeploymentManager:
    """Own controlled, shared, reusable, isolated, and local deployments."""

    DEPLOY_TIMEOUT_SECONDS: Final = 40 * 60

    def __init__(self, launcher: Launcher, config: DeploymentConfig) -> None:
        self.launcher = launcher
        self.config = config
        self._shared: Deployment | None = None
        self._reusable: Deployment | None = None
        self._quarantined_reusable: Deployment | None = None
        self._owned: list[Deployment] = []
        self._live: list[Deployment] = []
        self.created_count = 0

    def controlled(
        self,
        *args: str,
        config: DeploymentConfig | None = None,
    ) -> Deployment:
        return self._create(config or self.config, *args)

    def shared_live(self) -> Deployment:
        if self._shared is None:
            self._shared = self._create_live(self.config)
        return self._shared

    def reusable_live(self) -> Deployment:
        if self._quarantined_reusable is not None:
            message = "Reusable live deployment is quarantined after cleanup failure"
            raise RuntimeError(message)
        if self._reusable is None:
            self._reusable = self._create_live(self.config)
        return self._reusable

    def restore_reusable_ready(self, deployment: Deployment) -> None:
        if deployment is not self._reusable:
            message = "Deployment is not managed as reusable live"
            raise ValueError(message)

        def restore() -> None:
            if not deployment.has_status(StatusDatabaseReady):
                deployment.start()
            if (
                not deployment.has_status(StatusDatabaseReady)
                or not deployment.db_connectable()
            ):
                message = (
                    "Reusable live deployment could not be restored database-ready"
                )
                raise RuntimeError(message)

        try:
            restore()
        except Exception as error:
            log_tail = deployment.deployment_log_tail()
            self._quarantined_reusable = deployment
            try:
                self.release(deployment)
            except (OSError, RuntimeError) as cleanup_error:
                message = (
                    "Reusable live deployment restoration and cleanup failed\n"
                    f"restoration error: {error}\ncleanup error: {cleanup_error}\n"
                    f"deployment.log tail:\n{log_tail}"
                )
                raise RuntimeError(message) from error
            message = (
                "Reusable live deployment restoration failed; "
                "deployment was destroyed\n"
                f"restoration error: {error}\n"
                f"deployment.log tail:\n{log_tail}"
            )
            raise RuntimeError(message) from error

    def isolated_live(
        self,
        *args: str,
        config: DeploymentConfig | None = None,
        deploy: bool = True,
    ) -> Deployment:
        return self._create_live(config or self.config, *args, deploy=deploy)

    def verify_shared_ready(self, deployment: Deployment) -> None:
        if deployment is not self._shared:
            message = "Deployment is not managed as shared live"
            raise ValueError(message)
        if not deployment.has_status(StatusDatabaseReady):
            message = (
                "Shared live deployment was not left database-ready\n"
                f"deployment.log tail:\n{deployment.deployment_log_tail()}"
            )
            raise RuntimeError(message)

    def local(
        self,
        *args: str,
        config: DeploymentConfig | None = None,
        deploy: bool = True,
    ) -> Deployment:
        config = replace(
            config or self.config,
            infra="local",
            cluster_size=1,
            stackit_project_id=None,
        )
        return self._create_live(config, *args, deploy=deploy)

    def release(self, deployment: Deployment) -> None:
        if deployment not in self._owned:
            return
        if deployment is self._shared:
            self._shared = None
        if deployment is self._reusable:
            self._reusable = None
        if deployment in self._live:
            deployment.cleanup()
            self._live.remove(deployment)
        else:
            deployment.deployment_dir.cleanup()
        self._owned.remove(deployment)
        if deployment is self._quarantined_reusable:
            self._quarantined_reusable = None

    def close(self) -> None:
        errors: list[str] = []
        for deployment in self._owned.copy():
            try:
                self.release(deployment)
            except (OSError, RuntimeError) as error:
                errors.append(str(error))
        if errors:
            message = "Deployment cleanup failed:\n" + "\n".join(errors)
            raise RuntimeError(message)

    def _create(self, config: DeploymentConfig, *args: str) -> Deployment:
        deployment = Deployment(
            self.launcher,
            "--no-launcher-version-check",
            *args,
            config=config,
        )
        self.created_count += 1
        self._owned.append(deployment)
        return deployment

    def _create_live(
        self,
        config: DeploymentConfig,
        *args: str,
        deploy: bool = True,
    ) -> Deployment:
        deployment = self._create(config, *args)
        self._live.append(deployment)
        if not deploy:
            return deployment
        try:
            deployment.deploy(timeout=self.DEPLOY_TIMEOUT_SECONDS)
            is_ready = deployment.has_status(StatusDatabaseReady)
        except (TimeoutError, TimeoutExpired) as error:
            message = (
                f"Deploy command timed out after {self.DEPLOY_TIMEOUT_SECONDS}s\n"
                f"deployment.log tail:\n{deployment.deployment_log_tail()}"
            )
            self.release(deployment)
            raise RuntimeError(message) from error
        except BaseException:
            self.release(deployment)
            raise
        if not is_ready:
            message = (
                f"Expected status `{StatusDatabaseReady}` after `deploy`\n"
                f"deployment.log tail:\n{deployment.deployment_log_tail()}"
            )
            self.release(deployment)
            raise RuntimeError(message)
        return deployment
