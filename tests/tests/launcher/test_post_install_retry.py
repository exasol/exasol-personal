# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

import os
import shutil
import subprocess
from pathlib import Path

import pytest

pytestmark = pytest.mark.openspec("bucketfs-credentials")
WRAPPER = (
    Path(__file__).resolve().parents[3]
    / "assets/installation/ubuntu/monitor-installation.sh"
)
POST_INSTALL = "exasol_launcher_post_install.service"
TARGET = "exasol_launcher.target"


@pytest.fixture
def monitor_retry(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> Path:
    if os.name == "nt":
        pytest.skip("Ubuntu installer requires bash")
    wrapper = tmp_path / "wrapper.sh"
    wrapper.write_text(
        WRAPPER.read_text()
        .replace(
            "/opt/exasol_launcher/scripts/monitorInstallation.sh",
            str(tmp_path / "monitor.sh"),
        )
        .replace(
            "/var/lib/exasol_launcher/state/post_install.complete",
            str(tmp_path / "complete"),
        )
    )
    (tmp_path / "state").write_text("failed")
    (tmp_path / "monitor.sh").write_text("""#!/usr/bin/env bash
[[ -f "$TEST_DIR/complete" || $(cat "$TEST_DIR/state") != failed ]]
""")
    (tmp_path / "sudo").write_text('#!/usr/bin/env bash\nexec "$@"\n')
    (tmp_path / "systemctl").write_text("""#!/usr/bin/env bash
set -euo pipefail
case "$1" in
is-failed) [[ $(cat "$TEST_DIR/state") == failed ]] ;;
reset-failed)
    printf '%s\\n' "$*" >> "$TEST_DIR/calls"
    echo inactive > "$TEST_DIR/state" ;;
start)
    printf '%s\\n' "$*" >> "$TEST_DIR/calls"
    if [[ "${FAIL_RETRY:-}" == 1 ]]; then
        echo failed > "$TEST_DIR/state"
    else
        echo inactive > "$TEST_DIR/state"
        touch "$TEST_DIR/complete"
    fi ;;
*) exit 2 ;;
esac
""")
    for name in ("monitor.sh", "sudo", "systemctl"):
        (tmp_path / name).chmod(0o700)
    monkeypatch.setenv("TEST_DIR", str(tmp_path))
    monkeypatch.setenv("PATH", f"{tmp_path}{os.pathsep}{os.environ['PATH']}")
    return wrapper


def _monitor(wrapper: Path) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        [shutil.which("bash") or "bash", str(wrapper)],
        capture_output=True,
        text=True,
        check=False,
    )


def test_explicit_retry_recovers_failed_post_install(
    monitor_retry: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setenv("FAIL_RETRY", "1")
    assert _monitor(monitor_retry).returncode != 0
    assert not (monitor_retry.parent / "complete").exists()
    monkeypatch.delenv("FAIL_RETRY")
    assert _monitor(monitor_retry).returncode == 0
    assert (monitor_retry.parent / "complete").exists()
    assert (monitor_retry.parent / "calls").read_text().splitlines() == [
        f"reset-failed {POST_INSTALL} {TARGET}",
        f"start --no-block {POST_INSTALL} {TARGET}",
    ] * 2


@pytest.mark.parametrize("phase", ["complete", "activating", "inactive"])
def test_monitor_preserves_completed_and_running_phases(
    monitor_retry: Path,
    phase: str,
) -> None:
    if phase == "complete":
        (monitor_retry.parent / "complete").touch()
    else:
        (monitor_retry.parent / "state").write_text(phase)
    assert _monitor(monitor_retry).returncode == 0
    assert not (monitor_retry.parent / "calls").exists()
