import json
import os
import stat
import subprocess
import sys
from pathlib import Path
from subprocess import CompletedProcess

import pytest

pytestmark = [pytest.mark.e2e]


def _run(command: list[str]) -> CompletedProcess[str]:
    """Run a CLI command and require a clean exit."""
    return subprocess.run(
        command,
        capture_output=True,
        text=True,
        check=True,
        env=os.environ.copy(),
    )


def _first_infrastructure_preset_id_or_skip(exasol_path: str) -> str:
    """Return the first embedded infrastructure preset ID, or skip if none."""
    result = _run([exasol_path, "presets", "list", "--json"])
    data = json.loads(result.stdout)
    presets_list = data.get("infrastructures")
    if not isinstance(presets_list, list) or len(presets_list) == 0:
        pytest.skip("no infrastructure presets found")
    preset_id = presets_list[0].get("id")
    if not isinstance(preset_id, str) or preset_id.strip() == "":
        pytest.skip("first infrastructure preset has no id")
    return preset_id


@pytest.mark.skipif(
    sys.platform.startswith("win"),
    reason="POSIX permission semantics differ on Windows",
)
def test_init_in_non_writable_dir_fails_with_clear_error(
    exasol_path: str,
    tmp_path: Path,
) -> None:
    """Init into a non-writable directory must fail with an actionable error.

    Note: the launcher writes most state under --deployment-dir, plus a readline
    history file under $XDG_CACHE_HOME. This test exercises the former: the
    deployment directory itself is read-only.
    """
    # Given a deployment directory the user cannot write to
    deployment_dir = tmp_path / "locked"
    deployment_dir.mkdir()
    deployment_dir.chmod(stat.S_IRUSR | stat.S_IXUSR)

    infra_id = _first_infrastructure_preset_id_or_skip(exasol_path)

    # When init is invoked against it
    try:
        proc = subprocess.run(
            [
                exasol_path,
                "init",
                infra_id,
                "--deployment-dir",
                str(deployment_dir),
            ],
            capture_output=True,
            text=True,
            check=False,
        )

        # Then init exits non-zero with a message that references the path or
        # a permission problem, not a panic / Python traceback
        assert proc.returncode != 0
        combined = (proc.stdout + proc.stderr).lower()
        assert "panic:" not in combined
        assert "traceback" not in combined
        assert (
            "permission" in combined
            or "denied" in combined
            or "read-only" in combined
            or str(deployment_dir).lower() in combined
        ), f"Error did not name the path or permission cause: {combined!r}"
    finally:
        # Restore permissions so tmp_path cleanup can remove the directory
        deployment_dir.chmod(stat.S_IRWXU)


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

    infra_id = _first_infrastructure_preset_id_or_skip(exasol_path)
    _run(
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
    infra_id = _first_infrastructure_preset_id_or_skip(exasol_path)
    _run(
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
