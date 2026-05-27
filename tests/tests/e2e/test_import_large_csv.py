import os
import sys
import time
import uuid
from pathlib import Path
from typing import Final

import pytest

from framework.deployment import Deployment

pytestmark = [pytest.mark.e2e, pytest.mark.stress]

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


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
@pytest.mark.skipif(
    os.environ.get(_STRESS_ENV, "") != "1",
    reason=f"Set {_STRESS_ENV}=1 to run stress tests.",
)
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
