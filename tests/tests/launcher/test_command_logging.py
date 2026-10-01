# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

"""Regression tests for the shared DEBUG command logger."""

import logging
import subprocess
from pathlib import Path

import pytest

from tests import conftest as root_conftest


def test_debug_command_logging_redacts_password_values(
    exasol_path: str,
    caplog: pytest.LogCaptureFixture,
    monkeypatch: pytest.MonkeyPatch,
    tmp_path: Path,
) -> None:
    # Given a command line with password-bearing arguments
    def fake_popen(*_args: object, **_kwargs: object) -> object:
        return object()

    monkeypatch.setattr(root_conftest, "_real_popen", fake_popen)

    # When the shared subprocess logger formats the command line
    caplog.set_level(logging.DEBUG, logger="tests.commands")
    deployment_dir = tmp_path / "deployment"
    subprocess.Popen(
        [
            exasol_path,
            "init",
            "aws",
            "--db-password",
            "super-secret",
            "--adminui-password=admin-secret",
            "--password",
            "shell-secret",
            "--deployment-dir",
            str(deployment_dir),
        ]
    )

    # Then the sensitive values are redacted from the emitted debug log
    logged_text = caplog.text
    assert "super-secret" not in logged_text
    assert "admin-secret" not in logged_text
    assert "shell-secret" not in logged_text
    assert "<redacted>" in logged_text
    assert "--db-password" in logged_text
    assert "--adminui-password" in logged_text
    assert "--password" in logged_text
