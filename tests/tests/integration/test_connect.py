# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

import subprocess
from pathlib import Path

from .helpers import first_infrastructure_preset_id_or_skip, run_command


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
