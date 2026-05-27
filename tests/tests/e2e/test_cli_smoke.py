import subprocess
import sys

import pytest

pytestmark = [pytest.mark.e2e]


def test_unknown_flag_exits_nonzero_with_usage(exasol_path: str) -> None:
    """Unsupported flag must exit non-zero with usage and no stack trace."""
    # Given the launcher binary
    # When called with a flag that does not exist
    proc = subprocess.run(
        [exasol_path, "--definitely-not-a-flag"],
        capture_output=True,
        text=True,
        check=False,
    )

    # Then it exits non-zero and surfaces usage without a stack trace
    assert proc.returncode != 0
    combined = (proc.stdout + proc.stderr).lower()
    assert "unknown flag" in combined or "usage" in combined
    assert "traceback" not in combined
    assert "panic:" not in combined


@pytest.mark.skipif(
    not sys.platform.startswith("win"), reason="Windows-only shell-equivalence test"
)
def test_help_consistent_in_powershell_and_cmd(exasol_path: str) -> None:
    """--help output and exit code must match between PowerShell and CMD."""
    # Given the launcher on Windows
    # When --help is run via PowerShell and via cmd.exe
    powershell = subprocess.run(
        ["powershell.exe", "-NoProfile", "-Command", exasol_path, "--help"],  # noqa: S607
        capture_output=True,
        text=True,
        check=False,
    )
    cmd = subprocess.run(
        ["cmd.exe", "/c", exasol_path, "--help"],  # noqa: S607
        capture_output=True,
        text=True,
        check=False,
    )

    # Then both shells return success and the same help text
    assert powershell.returncode == 0
    assert cmd.returncode == 0
    assert powershell.stdout.strip() == cmd.stdout.strip()
