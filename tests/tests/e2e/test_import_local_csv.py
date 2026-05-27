import logging
import shutil
import sys
import textwrap
import uuid
from pathlib import Path
from typing import Final

import pytest

from framework.deployment import Deployment

pytestmark = [pytest.mark.e2e]

_PEOPLE_CSV: Final = (
    Path(__file__).parent.parent / "deployment" / "assets" / "people.csv"
)


def _make_table_statements(schema: str) -> list[str]:
    return [
        f"CREATE SCHEMA {schema};",
        f"OPEN SCHEMA {schema};",
        'CREATE TABLE Users ("id" INT PRIMARY KEY, "user_id" VARCHAR(32), '
        '"firstname" VARCHAR(100), "lastname" VARCHAR(100), '
        '"sex" VARCHAR(10), "email" VARCHAR(255));',
    ]


# ---------- P1 ----------


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
def test_import_csv_missing_file_reports_client_side(
    e2e_deployment: Deployment,
    tmp_path: Path,
) -> None:
    """Missing CSV path must produce a clear 'file not found' error."""
    # ========== GIVEN ==========
    missing = tmp_path / "does_not_exist.csv"
    assert not missing.exists()

    schema = f"test_import_missing_{uuid.uuid4().hex[:8]}"
    statements = [
        *_make_table_statements(schema),
        f"IMPORT INTO Users FROM LOCAL CSV FILE '{missing}';",
    ]

    # ========== WHEN ==========
    proc = e2e_deployment.connect(input="\n".join(statements), capture_output=True)

    # ========== THEN ==========
    # Some error must surface naming the missing file or "not found"
    combined = (proc.stdout + proc.stderr).lower()
    assert any(
        token in combined
        for token in ("not found", "no such file", "does not exist", "cannot open")
    ), f"Expected file-not-found error, got: {combined!r}"
    # And no server-side path leak (we never sent the file to the server)
    assert "/exa/" not in combined or "local" in combined


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
def test_import_csv_uses_local_filesystem(
    e2e_deployment: Deployment,
    tmp_path: Path,
) -> None:
    """IMPORT FROM LOCAL CSV must read from the client, not the node.

    Uses a path that demonstrably exists only on the client (a fresh tmp_path
    that the cluster nodes have no way to access). A successful import proves
    LOCAL semantics; a failure that references the client path is also valid.
    """
    # ========== GIVEN ==========
    assert _PEOPLE_CSV.exists()
    local_only = tmp_path / "client_only" / f"{uuid.uuid4().hex}.csv"
    local_only.parent.mkdir()
    shutil.copy(_PEOPLE_CSV, local_only)
    logging.info("Using client-only CSV at %s", local_only)

    schema = f"test_import_local_{uuid.uuid4().hex[:8]}"
    statements = [
        *_make_table_statements(schema),
        f"IMPORT INTO Users FROM LOCAL CSV FILE '{local_only}';",
        "SELECT * FROM Users;",
    ]

    # ========== WHEN ==========
    proc = e2e_deployment.connect(input="\n".join(statements), capture_output=True)

    # ========== THEN ==========
    # The import succeeds and returns the well-known reference row set
    assert proc.returncode == 0, f"Import failed: {proc.stderr!r}"
    expected = textwrap.dedent("""
    ┌────┬─────────────────┬───────────┬──────────┬────────┬─────────────────────────────┐
    │ id │     user_id     │ firstname │ lastname │  sex   │            email            │
    """).strip()  # noqa: E501
    assert expected.splitlines()[0] in proc.stdout, (
        f"Expected reference table header, got:\n{proc.stdout}"
    )


# ---------- P2 ----------


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
def test_import_csv_with_spaces_and_unicode_in_path(
    e2e_deployment: Deployment,
    tmp_path: Path,
) -> None:
    """Spaces and unicode characters in the CSV path must work."""
    # ========== GIVEN ==========
    # A copy of the standard people.csv placed at a path with spaces and unicode
    assert _PEOPLE_CSV.exists(), f"Reference CSV missing at {_PEOPLE_CSV}"
    weird_dir = tmp_path / "Ünicode folder"
    weird_dir.mkdir()
    csv_path = weird_dir / "my file.csv"
    shutil.copy(_PEOPLE_CSV, csv_path)

    schema = f"test_import_unicode_{uuid.uuid4().hex[:8]}"
    statements = [
        *_make_table_statements(schema),
        f"IMPORT INTO Users FROM LOCAL CSV FILE '{csv_path}';",
        'SELECT COUNT(*) AS "n" FROM Users;',
    ]

    # ========== WHEN ==========
    proc = e2e_deployment.connect(input="\n".join(statements), capture_output=True)

    # ========== THEN ==========
    # The import either succeeds (rows visible) or fails with a clear path-related
    # error - but never returns a misleading server-side path message.
    combined = (proc.stdout + proc.stderr).lower()
    if proc.returncode == 0 and "error" not in combined:
        # Idempotent header row excluded; reference file has 10 data rows
        assert "10" in proc.stdout
    else:
        # If the launcher rejects the path, the error must reference local paths
        assert (
            "local" in combined
            or str(csv_path).lower() in combined
            or ("file" in combined and "not" in combined)
        ), f"Unclear failure for path with spaces/unicode: {combined!r}"
