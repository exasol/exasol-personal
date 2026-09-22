# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

"""Tests for the Python launcher test framework."""

from pathlib import Path
from subprocess import CompletedProcess

from framework.launcher import Launcher


class InfoLauncher(Launcher):
    def __init__(self, stdout: str, stderr: str) -> None:
        super().__init__("unused")
        self.stdout = stdout
        self.stderr = stderr

    def info(self, deployment_dir: str, *args: str) -> CompletedProcess[str]:
        return CompletedProcess(
            args=["unused", "info", "--deployment-dir", deployment_dir, *args],
            returncode=0,
            stdout=self.stdout,
            stderr=self.stderr,
        )


def test_has_no_deployment_accepts_info_guidance_on_stderr(tmp_path: Path) -> None:
    # Given: info reports initialized state on stdout and next-step guidance on stderr.
    launcher = InfoLauncher(
        stdout="Exasol Personal Deployment Overview\nDeployment State: initialized\n",
        stderr="Ready for deployment. Run `exasol deploy` to apply it.\n",
    )

    # When / Then: the deployment framework accepts the current output contract.
    assert launcher.has_no_deployment(str(tmp_path))
