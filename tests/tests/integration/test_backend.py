# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

"""Backend resolution, preset compatibility, and build-size behavior (offline)."""

import os
import shutil
import subprocess
from pathlib import Path
from subprocess import CalledProcessError

import pytest

from tests.testcase_helpers import (
    export_preset,
    first_infrastructure_preset_id_or_skip,
    log_command,
    run_command,
)


@pytest.mark.launcher_tests
def test_unknown_backend_is_rejected(exasol_path: str, tmp_path: Path) -> None:
    # Given an exported infrastructure preset whose manifest declares an
    # unknown backend
    infra_id = first_infrastructure_preset_id_or_skip(exasol_path)
    infra_dir = tmp_path / "infra_export"
    infra_dir.mkdir()
    export_preset(exasol_path, infra_id, "infrastructure", str(infra_dir))

    manifest = infra_dir / "infrastructure.yaml"
    original = manifest.read_text()
    if "backend:" in original:
        patched = "\n".join(
            "backend: unknown" if line.strip().startswith("backend:") else line
            for line in original.splitlines()
        )
    else:
        patched = "backend: unknown\n" + original
    manifest.write_text(patched)

    install_dir = tmp_path / "install_export"
    install_dir.mkdir()
    export_preset(exasol_path, "ubuntu", "installation", str(install_dir))

    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()
    base = ["--deployment-dir", str(deployment_dir), "--no-launcher-version-check"]

    # When init resolves the backend from the manifest
    with pytest.raises(CalledProcessError) as exc:
        run_command([exasol_path, "init", str(infra_dir), str(install_dir), *base])

    # Then it fails fast citing an unknown deployment type and writes no state
    stderr = (exc.value.stderr or "").lower()
    assert "unknown deployment type" in stderr
    assert not (deployment_dir / ".exasolLauncherState.json").exists()


@pytest.mark.launcher_tests
def test_compatible_pair_succeeds(exasol_path: str, tmp_path: Path) -> None:
    # Given an empty deployment directory
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()

    # When a compatible infra/install pair is initialized
    result = run_command(
        [
            exasol_path,
            "init",
            "aws",
            "ubuntu",
            "--deployment-dir",
            str(deployment_dir),
            "--no-launcher-version-check",
        ]
    )

    # Then it succeeds
    assert result.returncode == 0


@pytest.mark.launcher_tests
def test_incompatible_pair_rejected_before_mutation(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given an empty deployment directory
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()

    # When an incompatible infra/install pair is initialized (aws + local)
    with pytest.raises(CalledProcessError) as exc:
        run_command(
            [
                exasol_path,
                "init",
                "aws",
                "local",
                "--deployment-dir",
                str(deployment_dir),
                "--no-launcher-version-check",
            ]
        )

    # Then it fails with a missing-capabilities error and writes nothing
    stderr = (exc.value.stderr or "").lower()
    assert "missing capabilities" in stderr
    assert list(deployment_dir.iterdir()) == []


@pytest.mark.launcher_tests
@pytest.mark.parametrize(
    "command",
    [["presets", "list"], ["init", "--help"]],
    ids=["presets-list", "init-help"],
)
def test_compatibility_matrix_rendered(exasol_path: str, command: list[str]) -> None:
    # When the command output is rendered
    output = run_command([exasol_path, *command]).stdout

    # Then it contains the compatibility matrix with yes/no cells
    assert "Compatibility matrix" in output
    assert "yes" in output
    assert "no" in output


PRESET_FLAGS = ("--cluster-size", "--instance-type")


@pytest.mark.launcher_tests
def test_help_subcommand_matches_help_flag(exasol_path: str) -> None:
    # When preset help is rendered both ways
    via_help = run_command([exasol_path, "help", "init", "aws"]).stdout
    via_flag = run_command([exasol_path, "init", "aws", "--help"]).stdout

    # Then both surface the same preset-specific flags
    for flag in PRESET_FLAGS:
        assert flag in via_help
        assert flag in via_flag


REPO_ROOT = Path(__file__).resolve().parents[3]


@pytest.mark.launcher_tests
@pytest.mark.skipif(shutil.which("task") is None, reason="`task` runner not installed")
@pytest.mark.skipif(shutil.which("go") is None, reason="Go toolchain not installed")
def test_debug_build_is_larger_than_release_build() -> None:
    # Given the built binary location
    binary = REPO_ROOT / "bin" / "exasol"

    def build(*, debug: bool) -> int:
        env = os.environ.copy()
        if debug:
            env["DEBUG_BUILD"] = "true"
        command = ["task", "build"]
        log_command(command)
        subprocess.run(
            command,
            cwd=REPO_ROOT,
            env=env,
            check=True,
            capture_output=True,
        )
        return binary.stat().st_size

    # When both build variants are produced
    release_size = build(debug=False)
    debug_size = build(debug=True)

    # Then the debug build (with symbols/DWARF) is larger than the stripped one
    assert debug_size > release_size

    # Restore the default release build for the rest of the suite.
    build(debug=False)
