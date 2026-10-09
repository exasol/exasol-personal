# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT
"""Launcher tests for version check functionality."""

import json
import os
import subprocess
import time
from pathlib import Path

import pytest
import requests

from .conftest import get_version_check_count


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


@pytest.mark.openspec("launcher-version-check")
def test_automatic_update_check_follows_only_success_and_skips_removed_state(
    exasol_path: str, mock_version_server: str, tmp_path: Path
) -> None:
    deployment_dir = tmp_path / "deployment"
    endpoint = f"{mock_version_server}/version-check"
    env = {**os.environ, "EXASOL_VERSION_CHECK_URL": endpoint}

    # Initialize without an implicit check, then enable checks in persisted state.
    initialized = subprocess.run(
        [
            exasol_path,
            "init",
            "aws",
            "--deployment-dir",
            str(deployment_dir),
            "--no-launcher-version-check",
        ],
        env=env,
        capture_output=True,
        text=True,
        check=True,
    )
    assert "EULA" in initialized.stderr
    assert get_version_check_count(mock_version_server) == 0
    state_file = deployment_dir / ".exasolLauncherState.json"
    state = json.loads(state_file.read_text())
    state["versionCheckEnabled"] = True
    state.pop("lastVersionCheck", None)
    state_file.write_text(json.dumps(state))

    # A command failure after shared startup must not request or show an update.
    failed = subprocess.run(
        [
            exasol_path,
            "status",
            "--timeout",
            "0",
            "--deployment-dir",
            str(deployment_dir),
        ],
        env=env,
        capture_output=True,
        text=True,
        check=False,
    )
    assert failed.returncode != 0
    assert "--timeout must be positive" in failed.stderr
    assert "A new version of Exasol Personal" not in failed.stderr
    assert get_version_check_count(mock_version_server) == 0

    # A successful command checks after its own output and leaves guidance last.
    succeeded = subprocess.run(
        [exasol_path, "status", "--deployment-dir", str(deployment_dir)],
        env=env,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        text=True,
        check=True,
    )
    assert "A new version of Exasol Personal is available: 9.9.9" in succeeded.stdout
    assert succeeded.stdout.rfind(
        "A new version of Exasol Personal"
    ) > succeeded.stdout.find("Status:")
    assert get_version_check_count(mock_version_server) == 1

    # A successful remove has no state to check and must not recreate the directory.
    state = json.loads(state_file.read_text())
    state.pop("lastVersionCheck", None)
    state_file.write_text(json.dumps(state))
    removed = subprocess.run(
        [
            exasol_path,
            "remove",
            "--auto-approve",
            "--deployment-dir",
            str(deployment_dir),
        ],
        env=env,
        capture_output=True,
        text=True,
        check=True,
    )
    assert "A new version of Exasol Personal" not in removed.stderr
    assert not deployment_dir.exists()
    assert get_version_check_count(mock_version_server) == 0


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
