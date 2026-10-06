# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

import json
import os
import subprocess
import sys
from pathlib import Path

import pytest

from .helpers import run_command

pytestmark = pytest.mark.skipif(sys.platform != "linux", reason="Linux host runtime")


@pytest.fixture(params=["initialized", "stopped"])
def host_deployment(
    exasol_path: str, tmp_path: Path, request: pytest.FixtureRequest
) -> Path:
    directory = tmp_path / "deployment"
    run_command(
        [
            exasol_path,
            "init",
            "local",
            "--deployment-dir",
            str(directory),
            "--no-launcher-version-check",
        ]
    )
    if request.param == "stopped":
        state_path = directory / ".exasolLauncherState.json"
        state = json.loads(state_path.read_text())
        state["currentWorkflowState"] = {"stopped": {}}
        state_path.write_text(json.dumps(state))
    return directory


@pytest.mark.parametrize("shell", ["configured", "empty", "unset"])
def test_linux_host_shell(
    exasol_path: str,
    host_deployment: Path,
    tmp_path: Path,
    shell: str,
) -> None:
    # Given
    environment = os.environ.copy()
    environment["HOST_SHELL_TEST_VALUE"] = "inherited"
    environment.pop("SHELL", None)
    if shell == "configured":
        executable = tmp_path / "chosen shell"
        executable.write_text('#!/bin/sh\nprintf "chosen\\n"\nexec /bin/sh\n')
        executable.chmod(0o700)
        environment["SHELL"] = str(executable)
    elif shell == "empty":
        environment["SHELL"] = ""
    # When
    result = subprocess.run(
        [exasol_path, "shell", "host", "--deployment-dir", str(host_deployment)],
        input='pwd\nprintf "%s\\n" "$HOST_SHELL_TEST_VALUE"\nprintf "diagnostic" >&2\n',
        cwd=tmp_path,
        env=environment,
        capture_output=True,
        text=True,
        check=True,
        timeout=10,
    )
    # Then
    expected = [str(tmp_path), "inherited"]
    if shell == "configured":
        expected.insert(0, "chosen")
    assert result.stdout.splitlines() == expected
    assert "diagnostic" in result.stderr


@pytest.mark.parametrize("exit_code", [0, 7])
def test_linux_host_command(
    exasol_path: str,
    host_deployment: Path,
    tmp_path: Path,
    exit_code: int,
) -> None:
    # Given
    arguments = ["with spaces", "", "$HOME", "; literal", "$(literal)"]
    environment = os.environ.copy()
    environment["SHELL"] = "/missing-shell"
    script = (
        "import json, os, sys; "
        "print(json.dumps([sys.argv[1:], sys.stdin.read(), os.getcwd()])); "
        f"sys.stderr.write('diagnostic'); sys.exit({exit_code})"
    )
    # When
    result = subprocess.run(
        [
            exasol_path,
            "shell",
            "host",
            "--deployment-dir",
            str(host_deployment),
            "--",
            sys.executable,
            "-c",
            script,
            *arguments,
        ],
        input="input-data",
        cwd=tmp_path,
        env=environment,
        capture_output=True,
        text=True,
        check=False,
        timeout=10,
    )
    # Then
    assert json.loads(result.stdout) == [arguments, "input-data", str(tmp_path)]
    assert "diagnostic" in result.stderr
    assert (result.returncode == 0) == (exit_code == 0)
