# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

"""SLC command input validation that needs no running deployment (offline)."""

import subprocess
from pathlib import Path
from typing import Final

import pytest

from .helpers import run_command

pytestmark = pytest.mark.openspec("custom-slc-language-identifiers")

LANGUAGE_RULE: Final = "must start with a letter or digit"


@pytest.fixture
def initialized_local_dir(exasol_path: str, tmp_path: Path) -> Path:
    """Return an initialized local deployment directory that was never deployed."""
    deployment_dir = tmp_path / "deployment"
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

    return deployment_dir


def _custom_install_with_language(
    exasol_path: str, deployment_dir: Path, language: str
) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        [
            exasol_path,
            "slc",
            "custom",
            "install",
            "--source",
            "container.tar.gz",
            "--alias",
            "MYLANG",
            f"--language={language}",
            "--deployment-dir",
            str(deployment_dir),
        ],
        capture_output=True,
        text=True,
        encoding="utf-8",
        check=False,
    )


@pytest.mark.parametrize("language", ["-bad", "a b", ".x", "cobol/", ""])
def test_custom_slc_rejects_invalid_language_identifier(
    exasol_path: str, initialized_local_dir: Path, language: str
) -> None:
    # When a custom SLC is installed with an identifier breaking the rule
    result = _custom_install_with_language(exasol_path, initialized_local_dir, language)

    # Then it is rejected with a message naming the identifier rule
    assert result.returncode != 0
    assert "invalid language identifier" in result.stderr
    assert LANGUAGE_RULE in result.stderr


@pytest.mark.parametrize("language", ["  Rust  ", "rust", "a.b-c_d", "Lua5"])
def test_custom_slc_accepts_language_identifiers_beyond_builtins(
    exasol_path: str, initialized_local_dir: Path, language: str
) -> None:
    # When a custom SLC is installed with a valid identifier outside python/java/r
    result = _custom_install_with_language(exasol_path, initialized_local_dir, language)

    # Then the identifier passes validation, and the command stops only at the
    # later check that the deployment was never deployed
    assert "invalid language identifier" not in result.stderr
    assert "deployment is not present" in result.stderr
