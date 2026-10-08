# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

"""Shared fixtures for launcher tests."""

import os
import socket
import subprocess
import threading
import time
from collections.abc import Iterator
from pathlib import Path

import pytest
import requests


@pytest.fixture
def mock_version_server() -> Iterator[str]:
    """Start mock version server and return its base URL."""
    port = "18080"
    mock_server_path = Path(__file__).parent.parent.parent / "mock_version_server.py"

    # Start the mock server (stdout/stderr suppressed)
    process = subprocess.Popen(
        ["python3", str(mock_server_path), "-port", port],  # noqa: S607
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )

    base_url = f"http://localhost:{port}"
    set_data_endpoint = (
        f"{base_url}/set-package-data"  # Configure mock server with test data
    )
    test_data = {
        "latestVersion": {
            "version": "9.9.9",
            "filename": "exasol-9.9.9.tar.gz",
            "url": "https://example.com/exasol-9.9.9.tar.gz",
            "size": 1234567890,
            "sha256": (
                "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
            ),
            "operatingSystem": "Linux",
            "architecture": "x86_64",
        }
    }

    # Wait for server to start by polling with POST requests
    max_attempts = 50  # 50 * 0.2s = 10 seconds max
    last_error = None
    for _attempt in range(max_attempts):
        try:
            # Check if process is still running
            if process.poll() is not None:
                stdout_bytes, stderr_bytes = process.communicate()
                msg = (
                    f"Mock server process exited unexpectedly\n"
                    f"Stdout: {stdout_bytes.decode('utf-8', errors='replace')}\n"
                    f"Stderr: {stderr_bytes.decode('utf-8', errors='replace')}"
                )
                raise RuntimeError(msg)

            response = requests.post(
                set_data_endpoint,
                json=test_data,
                timeout=1,
            )
            response.raise_for_status()
            break
        except requests.exceptions.RequestException as e:
            last_error = e
            time.sleep(0.2)
    else:
        stdout: str
        stderr: str
        if process.poll() is None:
            stdout_bytes, stderr_bytes = process.communicate(timeout=1)
            stdout = stdout_bytes.decode("utf-8", errors="replace")
            stderr = stderr_bytes.decode("utf-8", errors="replace")
        else:
            stdout = ""
            stderr = ""
        process.terminate()
        error_msg = (
            f"Mock server failed to start within timeout. "
            f"Last error: {last_error}\nStdout: {stdout}\nStderr: {stderr}"
        )
        raise RuntimeError(error_msg)

    try:
        yield base_url
    finally:
        # Clean up
        process.terminate()
        try:
            process.wait(timeout=5)
        except subprocess.TimeoutExpired:
            process.kill()


def get_version_check_count(base_url: str) -> int:
    """Get and reset the version check count from the mock server."""
    count_endpoint = f"{base_url}/version-check-count"
    response = requests.get(count_endpoint, timeout=1)
    response.raise_for_status()
    data = response.json()
    return int(data["count"])


@pytest.fixture
def unresponsive_db_port() -> Iterator[int]:
    """Accept TCP connections on a loopback port and never answer them."""
    listener = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    listener.bind(("127.0.0.1", 0))
    listener.listen(16)
    accepted: list[socket.socket] = []

    def accept_forever() -> None:
        while True:
            try:
                connection, _ = listener.accept()
            except OSError:
                return
            accepted.append(connection)

    threading.Thread(target=accept_forever, daemon=True).start()
    try:
        yield int(listener.getsockname()[1])
    finally:
        listener.close()
        for connection in accepted:
            connection.close()


@pytest.fixture
def hanging_podman_env(tmp_path: Path) -> dict[str, str]:
    """Return an environment whose `podman` never completes."""
    fake_bin = tmp_path / "hanging-podman-bin"
    fake_bin.mkdir()
    fake_podman = fake_bin / "podman"
    fake_podman.write_text("#!/bin/sh\nexec sleep 600\n")
    fake_podman.chmod(0o700)
    environment = os.environ.copy()
    environment["PATH"] = f"{fake_bin}{os.pathsep}{environment['PATH']}"

    return environment
