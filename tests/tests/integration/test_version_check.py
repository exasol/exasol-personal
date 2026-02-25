# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT
"""Integration tests for version check functionality."""

import json
import os
import subprocess
import time

import pytest
import requests


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
