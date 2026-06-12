import subprocess
import sys

import pytest

pytestmark = [pytest.mark.e2e]


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
