# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

"""Shared helpers for launcher and live tests."""

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
    export_preset,
    first_infrastructure_preset_id_or_skip,
    first_installation_preset_id_or_skip,
    installation_preset_id_or_skip,
    preset_id_or_skip,
    run_command,
)

__all__ = [
    "IS_MACOS_ARM",
    "export_preset",
    "first_infrastructure_preset_id_or_skip",
    "first_installation_preset_id_or_skip",
    "installation_preset_id_or_skip",
    "local_deploy_base_args",
    "preset_id_or_skip",
    "requires_macos_arm",
    "requires_posix_pty",
    "run_command",
    "run_in_local_vm",
]


# True only on the platform where real local VM deployments are supported.
IS_MACOS_ARM: bool = sys.platform == "darwin" and platform.machine().lower() in {
    "arm64",
    "aarch64",
}

# Skip a case unless we are on macOS Apple Silicon (real local VM required).
requires_macos_arm = pytest.mark.skipif(
    not IS_MACOS_ARM,
    reason="local VM deployments require macOS Apple Silicon",
)

# Skip a case that drives the CLI through a real pseudo-terminal. This is a
# property of the test, not of local deployments: it needs os.openpty and the
# POSIX termios/fcntl ioctls, which Windows does not provide.
requires_posix_pty = pytest.mark.skipif(
    sys.platform.startswith("win"),
    reason="test drives a pseudo-terminal, which requires POSIX openpty/termios",
)


def local_deploy_base_args(deployment_dir: str) -> list[str]:
    """Return the trailing args that point a command at a local deployment dir."""
    return ["--deployment-dir", deployment_dir, "--no-launcher-version-check"]


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
