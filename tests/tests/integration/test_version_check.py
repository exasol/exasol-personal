# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

"""Integration tests for version check functionality."""

import json
import os
import subprocess
import sys
import time
from pathlib import Path

import pytest
import requests

from tests.testcase_helpers import log_command, run_command


def test_version_check_latest(exasol_path: str, mock_version_server: str) -> None:
    """Test version --latest command with mock server (formatted output)."""
    endpoint = f"{mock_version_server}/version-check"

    # Verify server is responding before running the test (with retries)
    max_retries = 10
    for attempt in range(max_retries):
        try:
            requests.get(endpoint, timeout=1)
            # Server responded (even if 404), so it's alive
            break
        except requests.exceptions.RequestException as e:
            if attempt == max_retries - 1:
                pytest.fail(
                    f"Mock server not responding after {max_retries} attempts: {e}"
                )
            time.sleep(0.2)

    result = subprocess.run(
        [
            exasol_path,
            "version",
            "--latest",
        ],
        check=False,
        capture_output=True,
        text=True,
        env={**os.environ, "EXASOL_VERSION_CHECK_URL": endpoint},
    )

    assert result.returncode == 0, (
        f"Version check failed with return code {result.returncode}\n"
        f"Stdout: {result.stdout}\n"
        f"Stderr: {result.stderr}"
    )

    # Check that the formatted output contains expected version information
    output = result.stdout
    assert "Version: 9.9.9" in output
    assert "Operating System: Linux" in output
    assert "Architecture: x86_64" in output
    assert "Filename: exasol-9.9.9.tar.gz" in output
    assert "Size: 1234567890 bytes" in output
    assert "Download URL: https://example.com/exasol-9.9.9.tar.gz" in output
    assert "SHA256: abcdef1234567890" in output


def test_version_check_latest_json(exasol_path: str, mock_version_server: str) -> None:
    """Test version --latest --json command with mock server (JSON output)."""
    endpoint = f"{mock_version_server}/version-check"

    result = subprocess.run(
        [
            exasol_path,
            "version",
            "--latest",
            "--json",
        ],
        check=False,
        capture_output=True,
        text=True,
        env={**os.environ, "EXASOL_VERSION_CHECK_URL": endpoint},
    )

    assert result.returncode == 0, (
        f"Version check failed with return code {result.returncode}\n"
        f"Stdout: {result.stdout}\n"
        f"Stderr: {result.stderr}"
    )

    # Parse JSON output
    response_data = json.loads(result.stdout)

    # Verify JSON structure and content
    assert "latestVersion" in response_data
    latest = response_data["latestVersion"]
    expected_size = 1234567890
    expected_sha = "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
    assert latest["version"] == "9.9.9"
    assert latest["filename"] == "exasol-9.9.9.tar.gz"
    assert latest["url"] == "https://example.com/exasol-9.9.9.tar.gz"
    assert latest["size"] == expected_size
    assert latest["sha256"] == expected_sha
    assert latest["operatingSystem"] == "Linux"
    assert latest["architecture"] == "x86_64"


def test_version_check_latest_when_up_to_date(
    exasol_path: str, mock_version_server: str
) -> None:
    """Test version --latest when current version matches latest version."""
    endpoint = f"{mock_version_server}/version-check"

    # Get the current version
    version_result = subprocess.run(
        [exasol_path, "version"],
        check=False,
        capture_output=True,
        text=True,
    )
    assert version_result.returncode == 0
    current_version = version_result.stdout.strip()

    # Configure mock server to return the current version as latest
    set_data_endpoint = f"{mock_version_server}/set-package-data"
    test_data = {
        "latestVersion": {
            "version": current_version,
            "filename": f"exasol-{current_version}.tar.gz",
            "url": f"https://example.com/exasol-{current_version}.tar.gz",
            "size": 1234567890,
            "sha256": (
                "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
            ),
            "operatingSystem": "Linux",
            "architecture": "x86_64",
        }
    }

    response = requests.post(set_data_endpoint, json=test_data, timeout=5)
    response.raise_for_status()

    # Run version check
    result = subprocess.run(
        [
            exasol_path,
            "version",
            "--latest",
        ],
        check=False,
        capture_output=True,
        text=True,
        env={**os.environ, "EXASOL_VERSION_CHECK_URL": endpoint},
    )

    assert result.returncode == 0, (
        f"Version check failed with return code {result.returncode}\n"
        f"Stdout: {result.stdout}\n"
        f"Stderr: {result.stderr}"
    )

    # Check that output indicates we're using the latest version
    output = result.stdout
    assert "You are using the latest version" in output
    assert current_version in output
    # Should not contain the detailed version info when already up to date
    assert "Download URL:" not in output


def _current_version(exasol_path: str) -> str:
    command = [exasol_path, "version"]
    log_command(command)
    return subprocess.run(
        command, check=True, capture_output=True, text=True
    ).stdout.strip()


def _set_latest(base_url: str, version: str) -> None:
    payload = {
        "latestVersion": {
            "version": version,
            "filename": f"exasol-{version}.tar.gz",
            "url": f"https://example.com/exasol-{version}.tar.gz",
            "size": 1234567890,
            "sha256": ("abcdef1234567890" * 4),
            "operatingSystem": "Linux",
            "architecture": "x86_64",
        }
    }
    response = requests.post(f"{base_url}/set-package-data", json=payload, timeout=5)
    response.raise_for_status()


def _run_latest(
    exasol_path: str, base_url: str, *args: str
) -> subprocess.CompletedProcess[str]:
    command = [exasol_path, "version", "--latest", *args]
    log_command(command)
    return subprocess.run(
        command,
        check=True,
        capture_output=True,
        text=True,
        env={**os.environ, "EXASOL_VERSION_CHECK_URL": f"{base_url}/version-check"},
    )


@pytest.mark.launcher_tests
def test_older_reported_version_is_not_flagged_as_update(
    exasol_path: str, mock_version_server: str
) -> None:
    # Given the service reports an older official release than the build
    _set_latest(mock_version_server, "0.0.1")

    # When the update check runs
    output = _run_latest(exasol_path, mock_version_server).stdout

    # Then no newer version is offered
    assert "No newer official version is available" in output


@pytest.mark.launcher_tests
def test_equal_version_reports_latest(
    exasol_path: str, mock_version_server: str
) -> None:
    # Given the service reports the exact current version
    _set_latest(mock_version_server, _current_version(exasol_path))

    # When the update check runs
    output = _run_latest(exasol_path, mock_version_server).stdout

    # Then it reports we are on the latest version
    assert "You are using the latest version" in output


@pytest.mark.launcher_tests
def test_latest_json_is_valid_on_stdout(
    exasol_path: str, mock_version_server: str
) -> None:
    # Given a newer version is available
    _set_latest(mock_version_server, "9999.0.0")

    # When the update check runs with JSON output
    output = _run_latest(exasol_path, mock_version_server, "--json").stdout

    # Then stdout is valid JSON describing the latest version
    data = json.loads(output)
    assert data["latestVersion"]["version"] == "9999.0.0"


posix_only = pytest.mark.skipif(
    sys.platform.startswith("win"),
    reason="fake local runner script is POSIX-only",
)


FAKE_RUNNER = """#!/usr/bin/env sh
set -eu
case "$1" in
  init)
    mkdir -p vm vm-shared
    ;;
  start)
    shift
    printf '%s\\n' "$@" > start-args.txt
    mkdir -p vm vm-shared
    cat > vm-state.json <<'JSON'
{"vm_name":"exasol-local-vm","vm_ip":"192.168.64.2","ports":{"ssh":20022,"db":28563,"ui":0}}
JSON
    ;;
  stop)
    exit 0
    ;;
  *)
    echo "unexpected command: $1" >&2
    exit 2
    ;;
esac
"""


def _deploy_with_fake_runner(
    exasol_path: str,
    deployment_dir: Path,
    env: dict[str, str],
    init_extra_args: list[str],
) -> list[str]:
    """Init + deploy against the recording fake runner; return the start args."""
    run_command(
        [
            exasol_path,
            "init",
            "local",
            "--deployment-dir",
            str(deployment_dir),
            *init_extra_args,
        ],
        env=env,
    )

    runner_target = deployment_dir / "local" / "runtime" / "mac-runner-aarch64"
    runner_target.parent.mkdir(parents=True)
    runner_target.write_text(FAKE_RUNNER)
    runner_target.chmod(0o700)

    run_command(
        [exasol_path, "deploy", "--deployment-dir", str(deployment_dir)],
        env=env,
    )

    start_args_file = deployment_dir / "local" / "runtime" / "start-args.txt"
    return start_args_file.read_text().splitlines()


def _flag_value(args: list[str], flag: str) -> str | None:
    """Return the value of ``flag`` from recorded args (either arg style)."""
    for index, arg in enumerate(args):
        if arg == flag and index + 1 < len(args):
            return args[index + 1]
        if arg.startswith(f"{flag}="):
            return arg.split("=", 1)[1]
    return None


@posix_only
@pytest.mark.launcher_tests
def test_disabled_version_check_is_forwarded(exasol_path: str, tmp_path: Path) -> None:
    # Given a local deployment initialized with the version check disabled
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()
    env = {
        **os.environ,
        "EXASOL_LOCAL_ALLOW_UNSUPPORTED_PLATFORM": "1",
        "EXASOL_LOCAL_SKIP_DB_WAIT": "1",
    }

    # When the deployment starts the (fake) runner
    args = _deploy_with_fake_runner(
        exasol_path, deployment_dir, env, ["--no-launcher-version-check"]
    )

    # Then the runner receives the disabled flag and no URL/identity args
    assert _flag_value(args, "--version-check-enabled") == "false"
    assert _flag_value(args, "--version-check-url") is None
    assert _flag_value(args, "--version-check-identity") is None


@posix_only
@pytest.mark.launcher_tests
def test_enabled_version_check_forwards_url_and_identity(
    exasol_path: str, tmp_path: Path, mock_version_server: str
) -> None:
    # Given a local deployment with the version check enabled, pointed at the
    # mock version server so no real metrics endpoint is contacted
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()
    endpoint = f"{mock_version_server}/version-check"
    env = {
        **os.environ,
        "EXASOL_LOCAL_ALLOW_UNSUPPORTED_PLATFORM": "1",
        "EXASOL_LOCAL_SKIP_DB_WAIT": "1",
        "EXASOL_VERSION_CHECK_URL": endpoint,
    }

    # When the deployment starts the (fake) runner
    args = _deploy_with_fake_runner(exasol_path, deployment_dir, env, [])

    # Then the runner receives enabled=true, the check URL, and the
    # deployment's cluster identity
    assert _flag_value(args, "--version-check-enabled") == "true"
    assert _flag_value(args, "--version-check-url") == endpoint
    identity = _flag_value(args, "--version-check-identity")
    assert identity, "expected a non-empty cluster identity"
