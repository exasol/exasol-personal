# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

from pathlib import Path
from subprocess import CalledProcessError

import pytest

from .helpers import first_infrastructure_preset_id_or_skip, run_command


def test_install_requires_infra_preset_arg(exasol_path: str) -> None:
    # Given the install command

    # When it is invoked without arguments
    with pytest.raises(CalledProcessError) as exc:
        run_command([exasol_path, "install"])

    # Then it fails because the required infra preset argument is missing
    assert exc.value.returncode != 0
    assert (
        "requires" in (exc.value.stderr or "").lower()
        or "accepts" in (exc.value.stderr or "").lower()
    )


def test_install_help(exasol_path: str) -> None:
    # Given the install command

    # When help is invoked
    result = run_command([exasol_path, "install", "--help"])
    output: str = result.stdout.strip()

    # Then the output explains the command
    assert "Initialize and deploy Exasol in one step" in output

    # Then I see which preset names I can pass
    infra_id = first_infrastructure_preset_id_or_skip(exasol_path)
    assert "Available infrastructure presets:" in output
    assert infra_id in output
    assert "Available installation presets:" in output
    assert "exasol presets" in output


def test_install_executes_init_step(exasol_path: str, tmp_path: Path) -> None:
    # Given a non-empty deployment directory
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()
    (deployment_dir / "somefile.txt").write_text("x")

    # Given an infrastructure preset ID
    infra_id = first_infrastructure_preset_id_or_skip(exasol_path)

    # When the install command is invoked
    with pytest.raises(CalledProcessError) as excinfo:
        run_command(
            [
                exasol_path,
                "install",
                infra_id,
                "--deployment-dir",
                str(deployment_dir),
            ]
        )

    # Then it fails during initialization (proving init ran)
    assert excinfo.value.returncode != 0
    stderr = (excinfo.value.stderr or "").lower()
    assert "initialization failed" in stderr
    assert "deployment directory is not empty" in stderr
