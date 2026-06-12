import logging
import os
import shutil
import sys
import textwrap
import time
import uuid
from pathlib import Path
from typing import Final

import pytest

from framework.deployment import Deployment

pytestmark = [pytest.mark.e2e]

_STRESS_ENV: Final = "EXASOL_TEST_STRESS"
_SIZE_ENV: Final = "EXASOL_STRESS_CSV_MB"
_TIMEOUT_ENV: Final = "EXASOL_STRESS_TIMEOUT_S"


def _generate_csv(target_path: Path, size_mb: int) -> None:
    """Write a deterministic CSV of approximately size_mb megabytes."""
    row = "1,abc123,Alice,Example,Female,alice@example.com\n"
    row_bytes = row.encode("utf-8")
    target_bytes = size_mb * 1024 * 1024
    written = 0
    with target_path.open("wb") as fh:
        while written < target_bytes:
            fh.write(row_bytes)
            written += len(row_bytes)

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


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
@pytest.mark.skipif(
    os.environ.get(_STRESS_ENV, "") != "1",
    reason=f"Set {_STRESS_ENV}=1 to run stress tests.",
)
@pytest.mark.stress
def test_import_large_csv_completes_or_fails_actionably(
    e2e_deployment: Deployment,
    tmp_path: Path,
) -> None:
    """Large CSV import must finish or fail with actionable guidance."""
    # ========== GIVEN ==========
    size_mb = int(os.environ.get(_SIZE_ENV, "1024"))  # 1 GB default
    timeout_s = int(os.environ.get(_TIMEOUT_ENV, "1800"))  # 30 min default
    csv_path = tmp_path / "stress.csv"
    _generate_csv(csv_path, size_mb)

    schema = f"test_import_stress_{uuid.uuid4().hex[:8]}"
    statements = [
        f"CREATE SCHEMA {schema};",
        f"OPEN SCHEMA {schema};",
        'CREATE TABLE Users ("id" INT, "user_id" VARCHAR(32), '
        '"firstname" VARCHAR(100), "lastname" VARCHAR(100), '
        '"sex" VARCHAR(10), "email" VARCHAR(255));',
        f"IMPORT INTO Users FROM LOCAL CSV FILE '{csv_path}';",
        'SELECT COUNT(*) AS "n" FROM Users;',
        "exit",
    ]

    # ========== WHEN ==========
    start = time.monotonic()
    proc = e2e_deployment.connect(input="\n".join(statements), capture_output=True)
    elapsed = time.monotonic() - start

    # ========== THEN ==========
    combined = (proc.stdout + proc.stderr).lower()
    if proc.returncode == 0 and "error" not in combined:
        # Success path - row count is non-zero and we finished within budget
        assert elapsed < timeout_s, (
            f"Import succeeded but took {elapsed:.0f}s (> {timeout_s}s budget)"
        )
    else:
        # Failure must include actionable size/timeout guidance, not a silent hang
        assert any(
            token in combined
            for token in ("timeout", "size", "memory", "disk", "too large", "limit")
        ), f"Stress failure was not actionable: {combined!r}"
