# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

"""Offline connect CLI: argument/format validation and pre-deploy error handling."""

import subprocess
from pathlib import Path
from subprocess import CalledProcessError

import pytest

from .helpers import first_infrastructure_preset_id_or_skip, run_command


@pytest.mark.launcher_tests
def test_help_describes_invocation_json_contract(exasol_path: str) -> None:
    # When the connect help is rendered
    proc = run_command([exasol_path, "connect", "--help"])
    help_text = proc.stdout.lower()

    # Then it describes the rc5 invocation-level JSON contract
    assert "one json document" in help_text
    assert "statementtype" in help_text
    assert "rowsaffected" in help_text


@pytest.mark.launcher_tests
def test_invalid_json_format_is_rejected(exasol_path: str, tmp_path: Path) -> None:
    # Given an initialized deployment directory
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()
    run_command(
        [
            exasol_path,
            "init",
            "aws",
            "--deployment-dir",
            str(deployment_dir),
            "--no-launcher-version-check",
        ]
    )

    # When connect is asked for an unsupported JSON format
    with pytest.raises(CalledProcessError) as exc:
        run_command(
            [
                exasol_path,
                "connect",
                "--json=yaml",
                "-c",
                "SELECT 1",
                "--deployment-dir",
                str(deployment_dir),
            ]
        )

    # Then it lists the supported values
    assert "expected one of: pretty, compact" in (exc.value.stderr or "").lower()


@pytest.mark.launcher_tests
def test_command_and_file_are_mutually_exclusive(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given an initialized deployment directory and a SQL file
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()
    run_command(
        [
            exasol_path,
            "init",
            "aws",
            "--deployment-dir",
            str(deployment_dir),
            "--no-launcher-version-check",
        ]
    )
    script = tmp_path / "script.sql"
    script.write_text("SELECT 1;\n")

    # When both -c and -f are supplied
    with pytest.raises(CalledProcessError) as exc:
        run_command(
            [
                exasol_path,
                "connect",
                "-c",
                "SELECT 1",
                "-f",
                str(script),
                "--deployment-dir",
                str(deployment_dir),
            ]
        )

    # Then it is rejected as a mutually-exclusive-flags error
    assert "none of the others can be" in (exc.value.stderr or "").lower()


@pytest.mark.launcher_tests
def test_json_and_csv_are_mutually_exclusive(exasol_path: str, tmp_path: Path) -> None:
    # Given an initialized deployment directory
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()
    run_command(
        [
            exasol_path,
            "init",
            "aws",
            "--deployment-dir",
            str(deployment_dir),
            "--no-launcher-version-check",
        ]
    )

    # When both --json and --csv are supplied
    with pytest.raises(CalledProcessError) as exc:
        run_command(
            [
                exasol_path,
                "connect",
                "--json",
                "--csv",
                "-c",
                "SELECT 1",
                "--deployment-dir",
                str(deployment_dir),
            ]
        )

    # Then it is rejected as a mutually-exclusive-flags error
    assert "none of the others can be" in (exc.value.stderr or "").lower()


def test_connect_without_deploy_fails_clearly(
    exasol_path: str,
    tmp_path: Path,
) -> None:
    """Connect against an initialized-but-not-deployed dir must fail clearly.

    Simulates "deployment outputs missing/corrupt" by initializing a deployment
    (no cloud) and then attempting to connect before deploy has run.
    """
    # Given an initialized but not-yet-deployed deployment directory
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()

    infra_id = first_infrastructure_preset_id_or_skip(exasol_path)
    run_command(
        [
            exasol_path,
            "init",
            infra_id,
            "--deployment-dir",
            str(deployment_dir),
        ]
    )

    # When connect is invoked without a completed deploy
    proc = subprocess.run(
        [exasol_path, "connect", "--deployment-dir", str(deployment_dir)],
        capture_output=True,
        text=True,
        check=False,
    )

    # Then connect exits non-zero and explains the deployment is not ready
    assert proc.returncode != 0
    combined = (proc.stdout + proc.stderr).lower()
    assert "panic:" not in combined
    assert "traceback" not in combined
    assert any(
        token in combined
        for token in (
            "not deployed",
            "no deployment",
            "deploy first",
            "not ready",
            "deployment-info",
            "no workflow state",
            "ready for deployment",
        )
    ), f"Error did not signal missing deployment: {combined!r}"


def test_connect_with_corrupt_deployment_info_fails_clearly(
    exasol_path: str,
    tmp_path: Path,
) -> None:
    """Variant: corrupt deployment-info.txt must produce a clear error."""
    # Given an initialized deployment dir with a corrupted deployment-info.txt
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()
    infra_id = first_infrastructure_preset_id_or_skip(exasol_path)
    run_command(
        [
            exasol_path,
            "init",
            infra_id,
            "--deployment-dir",
            str(deployment_dir),
        ]
    )
    (deployment_dir / "deployment-info.txt").write_text(
        "this is not a valid deployment info file\n",
        encoding="utf-8",
    )

    # When connect is invoked
    proc = subprocess.run(
        [exasol_path, "connect", "--deployment-dir", str(deployment_dir)],
        capture_output=True,
        text=True,
        check=False,
    )

    # Then connect exits non-zero without a panic/traceback
    assert proc.returncode != 0
    combined = (proc.stdout + proc.stderr).lower()
    assert "panic:" not in combined
    assert "traceback" not in combined
