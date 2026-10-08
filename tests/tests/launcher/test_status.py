# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

"""Status command: bounded waits and timeout validation (offline)."""

import json
import os
import subprocess
import time
from pathlib import Path
from typing import Final

import pytest

from .helpers import record_running_local_deployment, run_command

pytestmark = pytest.mark.openspec("deployment-status-timeout")

DEFAULT_STATUS_TIMEOUT_SECONDS: Final = 5
STATUS_TIMEOUT_SLACK_SECONDS: Final = 3


def _init_local_deployment(exasol_path: str, deployment_dir: Path) -> None:
    run_command(
        [
            exasol_path,
            "init",
            "local",
            "--deployment-dir",
            str(deployment_dir),
            "--no-launcher-version-check",
        ]
    )


def _timed_status(
    exasol_path: str, deployment_dir: Path, env: dict[str, str], *extra: str
) -> tuple[float, subprocess.CompletedProcess[str]]:
    started = time.monotonic()
    result = subprocess.run(
        [
            exasol_path,
            "status",
            "--json",
            "--deployment-dir",
            str(deployment_dir),
            *extra,
        ],
        capture_output=True,
        text=True,
        encoding="utf-8",
        check=False,
        timeout=120,
        env=env,
    )

    return time.monotonic() - started, result


@pytest.mark.parametrize("timeout", ["0", "-1"])
def test_status_rejects_non_positive_timeout(
    exasol_path: str, tmp_path: Path, timeout: str
) -> None:
    # Given an initialized deployment directory
    deployment_dir = tmp_path / "deployment"
    _init_local_deployment(exasol_path, deployment_dir)

    # When status is given a non-positive bound
    result = subprocess.run(
        [
            exasol_path,
            "status",
            "--timeout",
            timeout,
            "--deployment-dir",
            str(deployment_dir),
        ],
        capture_output=True,
        text=True,
        encoding="utf-8",
        check=False,
    )

    # Then it is rejected with a message requiring a positive limit
    assert result.returncode != 0
    assert "--timeout must be positive" in result.stderr


@pytest.mark.skipif(os.name == "nt", reason="uses a POSIX fake Podman executable")
def test_status_returns_within_default_bound_for_unresponsive_deployment(
    exasol_path: str,
    tmp_path: Path,
    unresponsive_db_port: int,
    hanging_podman_env: dict[str, str],
) -> None:
    # Given a running local deployment whose runtime and database never answer
    deployment_dir = tmp_path / "deployment"
    _init_local_deployment(exasol_path, deployment_dir)
    record_running_local_deployment(deployment_dir, unresponsive_db_port)

    # When status runs without an explicit bound
    elapsed, result = _timed_status(exasol_path, deployment_dir, hanging_podman_env)

    # Then it still reports a status, within the default bound
    assert result.returncode == 0, result.stderr
    assert json.loads(result.stdout)["status"]
    assert elapsed < DEFAULT_STATUS_TIMEOUT_SECONDS + STATUS_TIMEOUT_SLACK_SECONDS


@pytest.mark.skipif(os.name == "nt", reason="uses a POSIX fake Podman executable")
def test_status_timeout_flag_sets_the_bound(
    exasol_path: str,
    tmp_path: Path,
    unresponsive_db_port: int,
    hanging_podman_env: dict[str, str],
) -> None:
    # Given a running local deployment whose runtime and database never answer
    deployment_dir = tmp_path / "deployment"
    _init_local_deployment(exasol_path, deployment_dir)
    record_running_local_deployment(deployment_dir, unresponsive_db_port)
    short_timeout: Final = 1
    long_timeout: Final = 9

    # When status runs with a short and a long explicit bound
    short_elapsed, short_result = _timed_status(
        exasol_path, deployment_dir, hanging_podman_env, "--timeout", str(short_timeout)
    )
    long_elapsed, long_result = _timed_status(
        exasol_path, deployment_dir, hanging_podman_env, "--timeout", str(long_timeout)
    )

    # Then each run waits for the configured bound, not the default
    assert short_result.returncode == 0, short_result.stderr
    assert long_result.returncode == 0, long_result.stderr
    assert short_elapsed < short_timeout + STATUS_TIMEOUT_SLACK_SECONDS
    assert long_elapsed >= long_timeout - 1
    assert long_elapsed < long_timeout + STATUS_TIMEOUT_SLACK_SECONDS
