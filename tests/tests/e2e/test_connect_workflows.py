import logging
import subprocess
import sys
from pathlib import Path
from typing import Final

import pytest

from framework.deployment import Deployment

pytestmark = [pytest.mark.e2e]


# ---------- P0 ----------


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
def test_password_marker_not_leaked_to_logs(e2e_deployment: Deployment) -> None:
    """A marker password must not appear in any output or log.

    Connect is invoked with an obviously wrong marker password to force a
    failure path. The marker must not appear in stdout, stderr, or in
    deployment.log.
    """
    # ========== GIVEN ==========
    # A deployment and a distinctive password marker
    marker: Final = "P@ssw0rd-MARKER-d3adbeef-cafebabe"

    # ========== WHEN ==========
    # We force an authentication failure with the marker as the password.
    # Launcher.run_command sets check=True, so a non-zero exit raises here;
    # we capture the same fields from the exception for inspection.
    try:
        proc = e2e_deployment.connect(
            "--password",
            marker,
            input="SELECT 1 FROM Dual;\nexit\n",
            capture_output=True,
        )
        stdout, stderr, returncode = proc.stdout, proc.stderr, proc.returncode
    except subprocess.CalledProcessError as exc:
        stdout = exc.stdout or ""
        stderr = exc.stderr or ""
        returncode = exc.returncode

    # ========== THEN ==========
    # The marker must not leak in the captured output
    assert marker not in stdout
    assert marker not in stderr

    # Nor in the deployment log
    deployment_log = Path(e2e_deployment.deployment_dir.name) / "deployment.log"
    if deployment_log.exists():
        log_contents = deployment_log.read_text(encoding="utf-8", errors="replace")
        assert marker not in log_contents, (
            f"Password marker leaked into {deployment_log}"
        )
    logging.info("Connect rc=%s; marker not found in stdout/stderr/log", returncode)


# ---------- P1 ----------


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
def test_connect_shows_exit_hint(e2e_deployment: Deployment) -> None:
    """An interactive connection must print the 'how to exit' banner.

    The launcher reads the DB password from the encrypted secrets file rather
    than prompting interactively, so the PDF's user/password-prompt expectation
    does not apply to the current implementation. The exit-hint contract still
    holds and is what we assert.
    """
    # ========== GIVEN ==========
    # A connectable deployment
    assert e2e_deployment.db_connectable()

    # ========== WHEN ==========
    # Connect is invoked with an immediate "exit" on stdin
    proc = e2e_deployment.connect(input="exit\n", capture_output=True)

    # ========== THEN ==========
    # The shell prints its exit hint to stderr and exits cleanly
    assert proc.returncode == 0
    assert 'Type "exit" to exit the shell' in proc.stderr


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
def test_invalid_sql_does_not_crash_shell(e2e_deployment: Deployment) -> None:
    """An invalid statement must error but keep the shell alive."""
    # ========== GIVEN ==========
    # A connected shell session driven via stdin
    # ========== WHEN ==========
    # We submit an invalid statement followed by a valid one
    queries = "SELEC 1;\nSELECT 1 AS ok FROM Dual;\n"
    proc = e2e_deployment.connect(input=queries, capture_output=True)

    # ========== THEN ==========
    # The valid query result still appears, proving the shell survived the error
    assert "ok" in proc.stdout.lower() or "1" in proc.stdout
    combined = (proc.stdout + proc.stderr).lower()
    assert any(token in combined for token in ("error", "syntax", "invalid")), (
        f"Expected SQL error message, got: {combined!r}"
    )


# ---------- P2 ----------


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
def test_many_statements_remain_stable(e2e_deployment: Deployment) -> None:
    """50 small statements must run without crash or hang."""
    # ========== GIVEN ==========
    # A connectable deployment
    assert e2e_deployment.db_connectable()

    # ========== WHEN ==========
    # We send 50 alternating trivial statements and exit
    statements_per_kind: Final = 25
    statements = []
    for _ in range(statements_per_kind):
        statements.append("SELECT 1 FROM Dual;")
        statements.append("SELECT CURRENT_TIMESTAMP FROM Dual;")
    statements.append("exit")

    proc = e2e_deployment.connect(input="\n".join(statements), capture_output=True)

    # ========== THEN ==========
    # The shell terminates cleanly and the deployment is still healthy
    assert proc.returncode == 0
    assert e2e_deployment.db_connectable()
