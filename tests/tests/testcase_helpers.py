# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

"""Shared helpers for launcher and live tests."""

import json
import os
import platform
import secrets
import shlex
import subprocess
import sys
from pathlib import Path

import pytest

# Re-export the existing integration helpers so the converted cases follow the
# same command-invocation conventions as the rest of the suite. Executed
# commands need no wrapper here: the root conftest logs every command at DEBUG.
from tests.launcher.helpers import (
    container_engine_args,
    export_preset,
    first_infrastructure_preset_id_or_skip,
    first_installation_preset_id_or_skip,
    installation_preset_id_or_skip,
    preset_id_or_skip,
    run_command,
)

__all__ = [
    "IS_MACOS_ARM",
    "assert_lifecycle_json_signal",
    "container_command",
    "container_engine",
    "container_engine_args",
    "export_preset",
    "first_infrastructure_preset_id_or_skip",
    "first_installation_preset_id_or_skip",
    "host_command",
    "installation_preset_id_or_skip",
    "local_deploy_base_args",
    "preset_id_or_skip",
    "requires_posix_pty",
    "run_command",
    "run_in_local_vm",
    "windows_podman_path",
]


# True only on the platform where real local VM deployments are supported.
IS_MACOS_ARM: bool = sys.platform == "darwin" and platform.machine().lower() in {
    "arm64",
    "aarch64",
}

# Skip a case that drives the CLI through a real pseudo-terminal. This is a
# property of the test, not of local deployments: it needs os.openpty and the
# POSIX termios/fcntl ioctls, which Windows does not provide.
requires_posix_pty = pytest.mark.skipif(
    sys.platform.startswith("win"),
    reason="test drives a pseudo-terminal, which requires POSIX openpty/termios",
)


def assert_lifecycle_json_signal(
    stdout: str, deployment_state: str, *, database_ready: bool
) -> None:
    assert json.loads(stdout) == {
        "deploymentState": deployment_state,
        "databaseReady": database_ready,
    }


def windows_podman_path() -> Path:
    """Return where the Windows host runtime installs Podman."""
    return Path(os.environ["PROGRAMFILES"]) / "RedHat" / "Podman" / "podman.exe"


def local_deploy_base_args(deployment_dir: str) -> list[str]:
    """Return the trailing args that point a command at a local deployment dir."""
    return ["--deployment-dir", deployment_dir, "--no-launcher-version-check"]


def host_command(
    exasol_path: str,
    deployment_dir: str | Path,
    *args: str,
    node: str | None = None,
    input_text: str | None = None,
) -> subprocess.CompletedProcess[str]:
    """Run a checked command where a deployment's containers run."""
    if node is None and sys.platform.startswith("linux"):
        # A direct-host runtime puts the containers on this machine.
        return subprocess.run(
            list(args),
            check=True,
            capture_output=True,
            text=True,
            input=input_text,
        )
    if node is None and sys.platform == "darwin":
        return run_in_local_vm(
            exasol_path, deployment_dir, shlex.join(args), input_text
        )

    return subprocess.run(
        [
            exasol_path,
            "shell",
            "host",
            "--deployment-dir",
            str(deployment_dir),
            *(["--node", node] if node else []),
            "--",
            *args,
        ],
        check=True,
        capture_output=True,
        text=True,
        input=input_text,
    )


def container_engine(deployment_dir: str | Path, node: str | None = None) -> str:
    """Return the container engine a deployment's containers run on.

    Direct-host deployments record their engine at first start; the macOS VM
    and cloud nodes always run Podman.
    """
    if node is not None or sys.platform == "darwin":
        return "podman"
    record = Path(deployment_dir) / "local" / "runtime" / "container-engine.json"
    if record.is_file():
        return str(json.loads(record.read_text())["engine"])
    return "podman"


def container_command(
    exasol_path: str,
    deployment_dir: str | Path,
    *args: str,
    node: str | None = None,
    input_text: str | None = None,
) -> subprocess.CompletedProcess[str]:
    """Run a checked container runtime command against a deployment."""
    return host_command(
        exasol_path,
        deployment_dir,
        container_engine(deployment_dir, node),
        *args,
        node=node,
        input_text=input_text,
    )


def run_in_local_vm(
    exasol_path: str,
    deployment_dir: str | Path,
    command: str,
    input_text: str | None = None,
) -> subprocess.CompletedProcess[str]:
    """Run a checked command through the interactive local VM shell."""
    import pty  # noqa: PLC0415 - only available on the macOS VM path

    deployment_path = Path(deployment_dir)
    shared = deployment_path / "local/runtime/vm-shared"
    shared.mkdir(parents=True, exist_ok=True)
    input_file = None
    if input_text is not None:
        input_file = shared / ("input-" + secrets.token_hex(16))
        input_file.write_text(input_text, encoding="utf-8")
        command += " < " + shlex.quote("/mnt/host/" + input_file.name)

    marker = "__VM_COMMAND_RESULT_" + secrets.token_hex(16) + "__"
    script = "\n".join(
        [
            "stty -echo",
            # An interactive shell prompts for every line it reads, which would
            # otherwise prefix the command output.
            "PS1=''",
            "PS2=''",
            f"printf '%s\\n' {shlex.quote(marker)}",
            command,
            "status=$?",
            f"printf '%s:%s\\n' {shlex.quote(marker)} \"$status\"",
            'exit "$status"',
        ]
    )
    launcher_command = [
        exasol_path,
        "shell",
        "host",
        "--deployment-dir",
        str(deployment_path),
    ]
    output = bytearray()
    input_data = (script + "\n").encode()
    input_sent = False

    def read_input(_fd: int) -> bytes:
        nonlocal input_sent
        if input_sent:
            return b""
        input_sent = True
        return input_data

    def read_output(fd: int) -> bytes:
        try:
            data = os.read(fd, 4096)
        except OSError:
            return b""
        output.extend(data)
        return data

    try:
        pty.spawn(launcher_command, master_read=read_output, stdin_read=read_input)
    finally:
        if input_file is not None:
            input_file.unlink(missing_ok=True)

    decoded = output.decode(errors="replace").replace("\r", "")
    result_start = decoded.rfind(marker + "\n")
    result_end = decoded.rfind(marker + ":")
    if result_start < 0 or result_end < result_start:
        message = "Local VM command did not return a result marker"
        raise AssertionError(message)
    command_output = decoded[result_start + len(marker) + 1 : result_end]
    returncode = int(decoded[result_end + len(marker) + 1 :].splitlines()[0])
    result = subprocess.CompletedProcess(
        launcher_command, returncode, command_output, ""
    )
    if returncode != 0:
        raise subprocess.CalledProcessError(
            returncode, launcher_command, output=command_output
        )
    return result
