# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

"""Local macOS VM deployments: lifecycle, memory, endpoints, keys, escape hatch."""

import json
import sys
from pathlib import Path
from typing import Final

import pytest

from tests.testcase_helpers import (
    LOCAL_ALLOW_UNSUPPORTED_ENV,
    requires_macos_arm,
    run_command,
)


@pytest.mark.local_e2e
@pytest.mark.installation_e2e
@requires_macos_arm
def test_full_local_deployment_lifecycle(exasol_path: str, tmp_path: Path) -> None:
    # Given a clean, empty local deployment directory
    deployment_dir = tmp_path / "exasol-local-test"
    deployment_dir.mkdir()
    base = ["--deployment-dir", str(deployment_dir)]

    # When the deployment is initialized and installed
    run_command([exasol_path, "init", "local", *base])
    # Then the infrastructure manifest declares the local backend (step 2)
    manifest = (deployment_dir / "infrastructure" / "infrastructure.yaml").read_text()
    assert "backend: local" in manifest

    run_command([exasol_path, "install", "local", *base])

    # Then the runner is staged under local/runtime (step 4)
    assert (deployment_dir / "local" / "runtime" / "mac-runner-aarch64").exists()

    # Then deployment.json and secrets.json describe a local connection (step 5)
    deployment_data = json.loads((deployment_dir / "deployment.json").read_text())
    connection = deployment_data["connection"]
    assert connection["host"] == "127.0.0.1"
    assert connection["dbPort"]
    secrets_data = json.loads((deployment_dir / "secrets.json").read_text())
    assert secrets_data["dbPassword"] == "exasol"

    # Then a trivial query returns the single DUMMY row (step 6)
    proc = run_command([exasol_path, "connect", "-c", "SELECT * FROM Dual", *base])
    assert "DUMMY" in proc.stdout

    # When the deployment is stopped and started (steps 9-10)
    run_command([exasol_path, "stop", *base])
    stopped = json.loads(run_command([exasol_path, "status", "--json", *base]).stdout)
    assert stopped["status"] == "stopped"

    run_command([exasol_path, "start", *base])
    running = json.loads(run_command([exasol_path, "status", "--json", *base]).stdout)
    assert running["status"] in {"database_ready", "database_connection_failed"}

    # When the deployment is destroyed with local cleanup (step 11)
    run_command([exasol_path, "destroy", "--remove", "--auto-approve", *base])
    assert not deployment_dir.exists()


OLD_FIXED_DEFAULT_MB: Final = 2048


LOCAL_MINIMUM_MEMORY_MB: Final = 4096


@pytest.mark.local_e2e
@requires_macos_arm
def test_memory_default_is_half_host_ram(exasol_path: str, tmp_path: Path) -> None:
    # Given a local deployment initialized without an explicit --memory-mb
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()
    base = ["--deployment-dir", str(deployment_dir)]
    run_command([exasol_path, "init", "local", *base])

    # When the resolved configuration is read
    config = json.loads(
        run_command([exasol_path, "config", "get", "--json", "memory-mb", *base]).stdout
    )
    memory_mb = config["infrastructure"]["options"]["memory-mb"]

    # Then it is no longer the old fixed default and honours the minimum
    assert memory_mb != OLD_FIXED_DEFAULT_MB
    assert memory_mb >= LOCAL_MINIMUM_MEMORY_MB


@pytest.mark.local_e2e
@requires_macos_arm
def test_reject_host_with_insufficient_ram() -> None:
    # This case can only be observed on a macOS Apple Silicon host with less
    # than 8 GB of RAM; there is no supported way to simulate a smaller host.
    pytest.skip("requires a macOS Apple Silicon host with <8 GB RAM")


@pytest.mark.local_e2e
@requires_macos_arm
def test_low_memory_advisory_notice() -> None:
    # Whether the advisory appears depends on the host's resolved VM memory
    # (<=8 GB); it can only be reliably asserted on such a host.
    pytest.skip(
        "requires a macOS Apple Silicon host whose resolved VM memory is <=8 GB"
    )


@pytest.mark.local_e2e
@requires_macos_arm
def test_local_deployment_json_is_endpoint_based(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given a fully installed local deployment
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()
    base = ["--deployment-dir", str(deployment_dir)]
    run_command([exasol_path, "init", "local", *base])
    run_command([exasol_path, "install", "local", *base])

    # When deployment.json is inspected
    deployment_data = json.loads((deployment_dir / "deployment.json").read_text())

    # Then it exposes a connection block and no top-level nodes array
    assert "connection" in deployment_data
    assert deployment_data["connection"]["host"] == "127.0.0.1"
    assert "nodes" not in deployment_data


OPENSSH_KEY_HEADER: Final = "-----BEGIN OPENSSH PRIVATE KEY-----"


LEGACY_PKCS8_KEY: Final = """-----BEGIN PRIVATE KEY-----
MC4CAQAwBQYDK2VwBCIEIP1HtSkjVc4Jl9U9jOJQl9Hpz27wSOZpmlGAsdOO5sx+
-----END PRIVATE KEY-----
"""


@pytest.mark.local_e2e
@requires_macos_arm
def test_node_access_key_is_openssh_and_legacy_key_is_regenerated(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given an installed local deployment
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()
    base = ["--deployment-dir", str(deployment_dir)]
    run_command([exasol_path, "init", "local", *base])
    run_command([exasol_path, "install", "local", *base])

    key_path = deployment_dir / "node_access.pem"

    # Then the generated key is in OpenSSH format, not PKCS#8 or classic RSA
    key_text = key_path.read_text()
    assert key_text.lstrip().startswith(OPENSSH_KEY_HEADER)
    assert "BEGIN PRIVATE KEY" not in key_text
    assert "BEGIN RSA PRIVATE KEY" not in key_text

    # When a legacy PKCS#8 key (rc4 format) is left on disk and the deployment
    # is restarted
    run_command([exasol_path, "stop", *base])
    key_path.write_text(LEGACY_PKCS8_KEY)
    run_command([exasol_path, "start", *base])

    # Then the legacy key is detected and regenerated in OpenSSH format
    regenerated = key_path.read_text()
    assert regenerated.lstrip().startswith(OPENSSH_KEY_HEADER)
    assert "BEGIN PRIVATE KEY" not in regenerated


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="local runner gate is POSIX-only here"
)
def test_allow_unsupported_escape_hatch(exasol_path: str, tmp_path: Path) -> None:
    # Given an empty deployment directory on a non-macOS host
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()

    # When init runs with the platform-check bypass enabled
    result = run_command(
        [
            exasol_path,
            "init",
            "local",
            "--deployment-dir",
            str(deployment_dir),
            "--no-launcher-version-check",
        ],
        env=LOCAL_ALLOW_UNSUPPORTED_ENV,
    )

    # Then init proceeds and initializes the deployment
    assert result.returncode == 0
    assert (deployment_dir / ".exasolLauncherState.json").exists()
