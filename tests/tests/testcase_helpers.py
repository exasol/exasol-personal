# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

"""Shared helpers for the ported manual test cases (``test_tc_*.py``).

Each ``test_tc_*.py`` module corresponds to exactly one manual test case
(``TC-*``). These modules were consolidated out of a standalone ``testcases/``
package into the ``integration``/``deployment``/``e2e``/``chaos`` suites; this
module provides the shared utilities they rely on.

The helpers here re-export the small integration-test utilities and add the
environment guards those manual cases assume (macOS Apple Silicon for local
deployments, a selected cloud ``--infra`` for cloud cases, real credentials,
etc.).
"""

import logging
import os
import platform
import shlex
import shutil
import sys
from pathlib import Path
from subprocess import CompletedProcess

import pytest

# Re-export the existing integration helpers so the converted cases follow the
# same command-invocation conventions as the rest of the suite. ``run_command``
# is wrapped below so every executed command is logged.
from tests.integration.helpers import (
    export_preset,
    first_infrastructure_preset_id_or_skip,
    first_installation_preset_id_or_skip,
    installation_preset_id_or_skip,
    preset_id_or_skip,
)
from tests.integration.helpers import run_command as _integration_run_command

__all__ = [
    "IS_MACOS_ARM",
    "LOCAL_ALLOW_UNSUPPORTED_ENV",
    "PRESET_FIXTURES_DIR",
    "copy_named_no_resource_presets",
    "export_preset",
    "first_infrastructure_preset_id_or_skip",
    "first_installation_preset_id_or_skip",
    "installation_preset_id_or_skip",
    "local_deploy_base_args",
    "log_command",
    "preset_id_or_skip",
    "requires_macos_arm",
    "requires_supported_local_platform",
    "run_command",
    "skip_unless_infra",
    "skip_without_cloud_deploy_optin",
]

_logger = logging.getLogger(__name__)


def log_command(command: list[str]) -> None:
    """Log a command line at DEBUG so ``-o log_cli_level=DEBUG`` prints it.

    Every case in this package routes its command executions through here (via
    ``run_command`` for launcher calls, or directly for ``subprocess.run``
    calls) so the exact commands are visible when debug logging is enabled.
    """
    _logger.debug("executing command: %s", shlex.join(command))


def run_command(
    command: list[str], env: dict[str, str] | None = None
) -> CompletedProcess[str]:
    """Log the command at DEBUG, then run it via the integration helper."""
    log_command(command)
    return _integration_run_command(command, env=env)


# Reuse the same no-resource preset fixtures as the integration suite. These
# presets deploy no external resources, so destroy/remove flows can run without
# any cloud provider or OpenTofu.
PRESET_FIXTURES_DIR: Path = Path(__file__).resolve().parent / "fixtures" / "presets"

# True only on the platform where real local VM deployments are supported.
IS_MACOS_ARM: bool = sys.platform == "darwin" and platform.machine().lower() in {
    "arm64",
    "aarch64",
}

# Test-only escape hatch that bypasses the local platform gate. Used by the
# cases that exercise local behaviour on CI runners without a real VM.
LOCAL_ALLOW_UNSUPPORTED_ENV: dict[str, str] = {
    **os.environ,
    "EXASOL_LOCAL_ALLOW_UNSUPPORTED_PLATFORM": "1",
}

# Skip a case unless we are on macOS Apple Silicon (real local VM required).
requires_macos_arm = pytest.mark.skipif(
    not IS_MACOS_ARM,
    reason="local VM deployments require macOS Apple Silicon",
)

# Skip a case that asserts the *rejection* on unsupported platforms when we are
# actually on the supported platform.
requires_supported_local_platform = pytest.mark.skipif(
    IS_MACOS_ARM,
    reason="platform-rejection case only applies off macOS Apple Silicon",
)


def skip_unless_infra(infra: str, *names: str) -> None:
    """Skip the current test unless ``--infra`` selected one of ``names``."""
    if infra not in names:
        pytest.skip(f"case targets infra {names!r}, but --infra={infra!r}")


def skip_without_cloud_deploy_optin() -> None:
    """Skip unless the caller has explicitly opted into real cloud provisioning.

    Cases that provision real (billable) cloud resources must not run during a
    plain ``pytest`` invocation. Set ``EXASOL_RUN_CLOUD_DEPLOY_CASES=1`` to
    enable them.
    """
    if os.getenv("EXASOL_RUN_CLOUD_DEPLOY_CASES") != "1":
        pytest.skip(
            "set EXASOL_RUN_CLOUD_DEPLOY_CASES=1 to run cases that provision "
            "real cloud resources"
        )


def local_deploy_base_args(deployment_dir: str) -> list[str]:
    """Return the trailing args that point a command at a local deployment dir."""
    return ["--deployment-dir", deployment_dir, "--no-launcher-version-check"]


def copy_named_no_resource_presets(
    tmp_path: Path,
    directory_name: str,
    infrastructure_name: str,
    installation_name: str,
) -> tuple[Path, Path]:
    """Materialize the named no-resource preset fixtures under ``tmp_path``.

    Returns the (infra_dir, install_dir) paths ready to pass to ``init``.
    """
    target_dir = tmp_path / directory_name
    shutil.copytree(PRESET_FIXTURES_DIR / "named-no-resource-template", target_dir)
    infra_dir = target_dir / "infra"
    install_dir = target_dir / "install"

    infrastructure_manifest = infra_dir / "infrastructure.yaml"
    infrastructure_manifest.write_text(
        infrastructure_manifest.read_text(encoding="utf-8").replace(
            "{{INFRASTRUCTURE_NAME}}", infrastructure_name
        ),
        encoding="utf-8",
    )
    installation_manifest = install_dir / "installation.yaml"
    installation_manifest.write_text(
        installation_manifest.read_text(encoding="utf-8").replace(
            "{{INSTALLATION_NAME}}", installation_name
        ),
        encoding="utf-8",
    )

    return infra_dir, install_dir
