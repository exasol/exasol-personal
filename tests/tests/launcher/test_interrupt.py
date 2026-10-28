# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

"""Launcher tests for interrupting deploy and destroy while OpenTofu is running.

The presets only run local commands, so no cloud resources are involved.
"""

import contextlib
import json
import os
import signal
import subprocess
import sys
import time
from collections.abc import Callable
from pathlib import Path
from typing import Any, Final

import pytest

from .helpers import run_command

pytestmark = [
    pytest.mark.openspec("deployment-lifecycle-recovery"),
    pytest.mark.skipif(
        sys.platform.startswith("win"),
        reason="POSIX signals and a POSIX shell for the OpenTofu commands are needed",
    ),
]

OPERATION_START_TIMEOUT_SECONDS: Final = 300
STOP_TIMEOUT_SECONDS: Final = 60
HEARTBEAT_QUIET_SECONDS: Final = 2.0
LAUNCHER_STATE_FILE: Final = ".exasolLauncherState.json"

INFRASTRUCTURE_MANIFEST: Final = """\
name: Endless local operation
description: Keeps one OpenTofu operation running until interrupted. Local only.
backend: tofu

compatibility:
  provides:
    - endless-operation

tofu:
  variablesFile: variables_public.tf
  varsOutputFile: vars.tfvars
"""

INFRASTRUCTURE_VARIABLES: Final = """\
variable "endless_operation" {
  description = "The OpenTofu operation that runs until interrupted: create or destroy"
  type        = string
  default     = "create"
}
"""

# The endless operation keeps touching a heartbeat file until OpenTofu stops
# it. Creating writes the files a completed deploy needs.
INFRASTRUCTURE_MAIN: Final = """\
locals {
  heartbeat = "while :; do touch heartbeat; sleep 0.1; done"
  deployment_info = jsonencode({
    deploymentId = "endless"
    connection   = { host = "127.0.0.1", dbPort = 1, username = "sys" }
  })
  secrets = jsonencode({ dbPassword = "unused" })
}

resource "terraform_data" "endless" {
  input = {
    operation = var.endless_operation
    heartbeat = local.heartbeat
  }

  provisioner "local-exec" {
    command = <<-EOT
      echo '${local.deployment_info}' > ../deployment.json
      echo '${local.secrets}' > ../secrets.json
      [ "${self.input.operation}" != create ] || ${self.input.heartbeat}
    EOT
  }

  provisioner "local-exec" {
    when    = destroy
    command = "[ '${self.input.operation}' != destroy ] || ${self.input.heartbeat}"
  }
}
"""

INSTALLATION_MANIFEST: Final = """\
name: No installation
description: Installs nothing.

compatibility:
  requires:
    - endless-operation

install: []
"""

# The OpenTofu operation that runs during each launcher command
TOFU_OPERATION: Final = {"deploy": "create", "destroy": "destroy"}


def _write_presets(tmp_path: Path) -> tuple[Path, Path]:
    infrastructure = tmp_path / "infrastructure-preset"
    infrastructure.mkdir()
    (infrastructure / "infrastructure.yaml").write_text(INFRASTRUCTURE_MANIFEST)
    (infrastructure / "variables_public.tf").write_text(INFRASTRUCTURE_VARIABLES)
    (infrastructure / "main.tf").write_text(INFRASTRUCTURE_MAIN)
    installation = tmp_path / "installation-preset"
    installation.mkdir()
    (installation / "installation.yaml").write_text(INSTALLATION_MANIFEST)
    return infrastructure, installation


def _deployment_ready_for(exasol_path: str, tmp_path: Path, operation: str) -> Path:
    """Return a deployment on which `operation` keeps OpenTofu running."""
    infrastructure, installation = _write_presets(tmp_path)
    deployment_dir = tmp_path / "deployment"
    run_command(
        [
            exasol_path,
            "init",
            str(infrastructure),
            str(installation),
            "--deployment-dir",
            str(deployment_dir),
            "--auto-approve",
            "--endless-operation",
            TOFU_OPERATION[operation],
        ]
    )
    if operation == "destroy":
        run_command(
            [
                exasol_path,
                "deploy",
                "--deployment-dir",
                str(deployment_dir),
                "--auto-approve",
            ]
        )
    return deployment_dir


def _wait_for(condition: Callable[[], bool], timeout: float, message: str) -> None:
    deadline = time.monotonic() + timeout
    while not condition():
        if time.monotonic() > deadline:
            pytest.fail(message)
        time.sleep(0.1)


def _heartbeat_is_quiet(heartbeat: Path) -> bool:
    """Whether the OpenTofu operation stopped touching its heartbeat file."""
    before = heartbeat.stat().st_mtime_ns
    time.sleep(HEARTBEAT_QUIET_SECONDS)
    return heartbeat.stat().st_mtime_ns == before


def _send_to_launcher(process: subprocess.Popen[bytes], sig: signal.Signals) -> None:
    """Signal only the launcher, as `kill <pid>` or a supervisor does."""
    os.kill(process.pid, sig)


def _send_to_process_group(
    process: subprocess.Popen[bytes], sig: signal.Signals
) -> None:
    """Signal the whole process group, as Ctrl+C in a terminal does."""
    os.killpg(process.pid, sig)


def _json_output(
    exasol_path: str, command: str, deployment_dir: Path
) -> dict[str, Any]:
    result = run_command(
        [exasol_path, command, "--deployment-dir", str(deployment_dir), "--json"]
    )
    output: dict[str, Any] = json.loads(result.stdout)
    return output


@pytest.mark.parametrize("operation", ["deploy", "destroy"])
@pytest.mark.parametrize(
    "sig", [signal.SIGINT, signal.SIGTERM], ids=["SIGINT", "SIGTERM"]
)
@pytest.mark.parametrize(
    "deliver",
    [
        pytest.param(_send_to_launcher, id="launcher-only"),
        pytest.param(_send_to_process_group, id="process-group"),
    ],
)
def test_interrupted_operation_stops_opentofu_and_is_recorded(
    exasol_path: str,
    tmp_path: Path,
    operation: str,
    sig: signal.Signals,
    deliver: Callable[[subprocess.Popen[bytes], signal.Signals], None],
) -> None:
    """An interrupted deploy or destroy stops OpenTofu before the launcher exits.

    OpenTofu records what it changed while stopping, so the launcher must neither
    exit before it nor leave it running. The deployment is then reported as
    interrupted during that operation.
    """
    # Given: a deploy or destroy whose OpenTofu operation is running
    deployment_dir = _deployment_ready_for(exasol_path, tmp_path, operation)
    heartbeat = deployment_dir / "infrastructure" / "heartbeat"
    process = subprocess.Popen(
        [
            exasol_path,
            operation,
            "--deployment-dir",
            str(deployment_dir),
            "--auto-approve",
        ],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
        start_new_session=True,
    )
    try:
        _wait_for(
            heartbeat.exists,
            OPERATION_START_TIMEOUT_SECONDS,
            f"the OpenTofu operation of {operation} did not start",
        )

        # When: the operation is interrupted
        deliver(process, sig)
        returncode = process.wait(timeout=STOP_TIMEOUT_SECONDS)

        # Then: the launcher exits with an error only after OpenTofu stopped
        assert returncode != 0
        assert _heartbeat_is_quiet(heartbeat), (
            "the OpenTofu operation was still running after the launcher exited"
        )

        # Then: the interruption is persisted with the interrupted operation
        state = json.loads((deployment_dir / LAUNCHER_STATE_FILE).read_text())
        interrupted = state["currentWorkflowState"]["interrupted"]
        assert interrupted["interruptedDuringOperation"] == operation

        # Then: status and info report the persisted state
        status = _json_output(exasol_path, "status", deployment_dir)
        assert status["status"] == "interrupted"
        assert f"Interrupted during {operation}" in status["message"]
        info = _json_output(exasol_path, "info", deployment_dir)
        assert info["deploymentState"] == "interrupted"
    finally:
        # Also ends an OpenTofu left running in the group after a failure
        with contextlib.suppress(ProcessLookupError):
            os.killpg(process.pid, signal.SIGKILL)
        process.wait()
