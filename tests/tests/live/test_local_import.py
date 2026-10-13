# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

import csv
import logging
import os
import shutil
import subprocess
import sys
import textwrap
import time
import uuid
from pathlib import Path
from typing import Final

import pytest

from framework.deployment import Deployment

pytestmark = pytest.mark.openspec("connect-sql-input")

_STRESS_ENV: Final = "EXASOL_TEST_STRESS"
_SIZE_ENV: Final = "EXASOL_STRESS_CSV_MB"
_TIMEOUT_ENV: Final = "EXASOL_STRESS_TIMEOUT_S"


def _decode_stream(stream: str | bytes | None) -> str:
    """Normalize a possibly-missing subprocess stream to text."""
    if stream is None:
        return ""
    if isinstance(stream, bytes):
        return stream.decode("utf-8", errors="replace")
    return stream


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


_PEOPLE_CSV: Final = Path(__file__).parent / "assets" / "people.csv"
_PARQUET_FIXTURE: Final = (
    Path(__file__).parent / "assets" / "local_parquet_fixture.parquet"
)


def _make_table_statements(schema: str) -> list[str]:
    return [
        f"CREATE SCHEMA {schema};",
        f"OPEN SCHEMA {schema};",
        (
            'CREATE TABLE Users ("id" INT PRIMARY KEY, "user_id" VARCHAR(32), '
            '"firstname" VARCHAR(100), "lastname" VARCHAR(100), '
            '"sex" VARCHAR(10), "email" VARCHAR(255));'
        ),
    ]


# ---------- P1 ----------


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
def test_import_csv_missing_file_reports_client_side(
    shared_live_deployment: Deployment,
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
    proc = shared_live_deployment.connect(
        input="\n".join(statements), capture_output=True
    )

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
    shared_live_deployment: Deployment,
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
    proc = shared_live_deployment.connect(
        input="\n".join(statements), capture_output=True
    )

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


def _parquet_rows_statements(schema: str) -> list[str]:
    return [
        f"CREATE SCHEMA {schema};",
        f"OPEN SCHEMA {schema};",
        'CREATE TABLE ParquetRows ("id" BIGINT, "name" VARCHAR(32));',
    ]


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
@pytest.mark.openspec("local-parquet-import")
@pytest.mark.parametrize("mode", ["stdin", "file"])
def test_import_parquet_uses_local_filesystem(
    shared_live_deployment: Deployment,
    tmp_path: Path,
    mode: str,
) -> None:
    """IMPORT INTO ... FROM LOCAL PARQUET must read a client-local file."""
    # ========== GIVEN ==========
    assert _PARQUET_FIXTURE.exists()
    local_only = tmp_path / "client_only" / f"{uuid.uuid4().hex}.parquet"
    local_only.parent.mkdir()
    shutil.copy(_PARQUET_FIXTURE, local_only)

    schema = f"test_import_parquet_{mode}_{uuid.uuid4().hex[:8]}"
    script = "\n".join(
        [
            *_parquet_rows_statements(schema),
            f"IMPORT INTO ParquetRows FROM LOCAL PARQUET FILE '{local_only}';",
            'SELECT "id", "name" FROM ParquetRows ORDER BY "id";',
        ]
    )

    # ========== WHEN ==========
    if mode == "file":
        sql_file = tmp_path / "import.sql"
        sql_file.write_text(script)
        proc = shared_live_deployment.connect(
            "--file", str(sql_file), capture_output=True
        )
    else:
        proc = shared_live_deployment.connect(input=script, capture_output=True)

    # ========== THEN ==========
    assert proc.returncode == 0, f"Parquet import failed: {proc.stderr!r}"
    assert "alpha" in proc.stdout
    assert "beta" in proc.stdout
    assert "gamma" in proc.stdout


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
@pytest.mark.openspec("local-parquet-import")
def test_import_parquet_missing_file_reports_client_side(
    shared_live_deployment: Deployment,
    tmp_path: Path,
) -> None:
    """A missing local Parquet file must fail without a successful import."""
    # ========== GIVEN ==========
    missing = tmp_path / "does_not_exist.parquet"
    assert not missing.exists()

    schema = f"test_import_parquet_missing_{uuid.uuid4().hex[:8]}"
    statements = [
        f"CREATE SCHEMA {schema};",
        f"OPEN SCHEMA {schema};",
        'CREATE TABLE ParquetRows ("id" BIGINT, "name" VARCHAR(32));',
        f"IMPORT INTO ParquetRows FROM LOCAL PARQUET FILE '{missing}';",
    ]

    # ========== WHEN ==========
    with pytest.raises(subprocess.CalledProcessError) as error_info:
        shared_live_deployment.connect(
            "--command", "\n".join(statements), capture_output=True
        )

    # ========== THEN ==========
    error = error_info.value
    combined = (_decode_stream(error.stdout) + _decode_stream(error.stderr)).lower()
    assert error.returncode != 0
    assert any(
        token in combined
        for token in ("not found", "no such file", "does not exist", "cannot open")
    ), f"Expected file-not-found error, got: {combined!r}"


def _client_only_parquet(tmp_path: Path) -> Path:
    local_only = tmp_path / "client_only" / f"{uuid.uuid4().hex}.parquet"
    local_only.parent.mkdir()
    shutil.copy(_PARQUET_FIXTURE, local_only)
    return local_only


def _inferred_table_statement(table: str, path: Path, *, replace: bool = False) -> str:
    create = "CREATE OR REPLACE TABLE" if replace else "CREATE TABLE"
    return f"{create} {table} AS (IMPORT FROM LOCAL PARQUET FILE '{path}');"


def _drop_schema(deployment: Deployment, schema: str) -> None:
    deployment.connect(
        "--command", f"DROP SCHEMA IF EXISTS {schema} CASCADE;", capture_output=True
    )


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
@pytest.mark.openspec("local-parquet-import")
def test_create_table_from_local_parquet_infers_schema(
    reusable_live_deployment: Deployment,
    tmp_path: Path,
) -> None:
    """CREATE TABLE ... AS (IMPORT FROM LOCAL PARQUET ...) creates a filled table."""
    # ========== GIVEN ==========
    local_only = _client_only_parquet(tmp_path)
    schema = f"PARQUET_INFER_{uuid.uuid4().hex[:8]}"
    try:
        statements = [
            f"CREATE SCHEMA {schema};",
            _inferred_table_statement(f"{schema}.parquet_rows", local_only),
            f'SELECT "id", "name" FROM {schema}.parquet_rows ORDER BY "id";',  # noqa: S608 - test-generated identifier
        ]

        # ========== WHEN ==========
        proc = reusable_live_deployment.connect(
            "--command", "\n".join(statements), capture_output=True
        )

        # ========== THEN ==========
        assert proc.returncode == 0, f"Inferred import failed: {proc.stderr!r}"
        assert "alpha" in proc.stdout
        assert "beta" in proc.stdout
        assert "gamma" in proc.stdout
        column_query = (
            "SELECT COLUMN_NAME, COLUMN_TYPE FROM SYS.EXA_ALL_COLUMNS "  # noqa: S608 - test-generated identifier
            f"WHERE COLUMN_SCHEMA = '{schema.upper()}' "
            "AND COLUMN_TABLE = 'PARQUET_ROWS' "
            "ORDER BY COLUMN_ORDINAL_POSITION;"
        )
        columns = reusable_live_deployment.connect(
            "--csv", "--command", column_query, capture_output=True
        )
        assert list(csv.reader(columns.stdout.splitlines()))[1:] == [
            ["id", "DECIMAL(19,0)"],
            ["name", "VARCHAR(2000000) UTF8"],
        ]
    finally:
        _drop_schema(reusable_live_deployment, schema)


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
@pytest.mark.openspec("local-parquet-import")
def test_create_or_replace_table_from_local_parquet_replaces_table(
    reusable_live_deployment: Deployment,
    tmp_path: Path,
) -> None:
    """CREATE OR REPLACE replaces an existing table with the file's columns and rows."""
    # ========== GIVEN ==========
    local_only = _client_only_parquet(tmp_path)
    schema = f"PARQUET_REPLACE_{uuid.uuid4().hex[:8]}"
    try:
        table = f"{schema}.parquet_rows"
        statements = [
            f"CREATE SCHEMA {schema};",
            f"CREATE TABLE {table} (old_column VARCHAR(10));",
            f"INSERT INTO {table} VALUES ('stale');",  # noqa: S608 - test-generated identifier
            _inferred_table_statement(table, local_only, replace=True),
            f'SELECT "id", "name" FROM {table} ORDER BY "id";',  # noqa: S608 - test-generated identifier
        ]

        # ========== WHEN ==========
        proc = reusable_live_deployment.connect(
            "--command", "\n".join(statements), capture_output=True
        )

        # ========== THEN ==========
        assert proc.returncode == 0, f"Replacing import failed: {proc.stderr!r}"
        assert "gamma" in proc.stdout
        assert "stale" not in proc.stdout
    finally:
        _drop_schema(reusable_live_deployment, schema)


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
@pytest.mark.openspec("local-parquet-import")
def test_create_table_from_local_parquet_rejects_existing_table(
    reusable_live_deployment: Deployment,
    tmp_path: Path,
) -> None:
    """CREATE TABLE without OR REPLACE fails and keeps an existing table."""
    # ========== GIVEN ==========
    local_only = _client_only_parquet(tmp_path)
    schema = f"PARQUET_EXISTS_{uuid.uuid4().hex[:8]}"
    try:
        table = f"{schema}.parquet_rows"
        reusable_live_deployment.connect(
            "--command",
            f"CREATE SCHEMA {schema}; CREATE TABLE {table} (old_column VARCHAR(10)); "  # noqa: S608 - test-generated identifier
            f"INSERT INTO {table} VALUES ('kept');",
            capture_output=True,
        )

        statement = _inferred_table_statement(table, local_only)

        # ========== WHEN ==========
        with pytest.raises(subprocess.CalledProcessError) as error_info:
            reusable_live_deployment.connect(
                "--command", statement, capture_output=True
            )

        # ========== THEN ==========
        error = error_info.value
        combined = _decode_stream(error.stdout) + _decode_stream(error.stderr)
        assert "already exists" in combined, (
            f"Expected existing-table error: {combined!r}"
        )
        proc = reusable_live_deployment.connect(
            "--command",
            f"SELECT old_column FROM {table};",  # noqa: S608 - test-generated identifier
            capture_output=True,
        )
        assert "kept" in proc.stdout
    finally:
        _drop_schema(reusable_live_deployment, schema)


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
@pytest.mark.openspec("local-parquet-import")
def test_create_table_from_missing_local_parquet_fails(
    reusable_live_deployment: Deployment,
    tmp_path: Path,
) -> None:
    """A missing file fails the inferred import and creates no table."""
    # ========== GIVEN ==========
    missing = tmp_path / "does_not_exist.parquet"
    schema = f"PARQUET_MISSING_{uuid.uuid4().hex[:8]}"
    try:
        reusable_live_deployment.connect(
            "--command", f"CREATE SCHEMA {schema};", capture_output=True
        )

        statement = _inferred_table_statement(f"{schema}.parquet_rows", missing)

        # ========== WHEN ==========
        with pytest.raises(subprocess.CalledProcessError) as error_info:
            reusable_live_deployment.connect(
                "--command", statement, capture_output=True
            )

        # ========== THEN ==========
        error = error_info.value
        combined = _decode_stream(error.stdout) + _decode_stream(error.stderr)
        assert str(missing) in combined, f"Expected the missing path in: {combined!r}"
        # The table name is still free, so no table was created.
        reusable_live_deployment.connect(
            "--command",
            f"CREATE TABLE {schema}.parquet_rows (x INT);",
            capture_output=True,
        )
    finally:
        _drop_schema(reusable_live_deployment, schema)


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
@pytest.mark.openspec("local-parquet-import")
def test_failed_replace_from_local_parquet_keeps_existing_table(
    reusable_live_deployment: Deployment,
    tmp_path: Path,
) -> None:
    """A failed CREATE OR REPLACE import keeps the existing table and its rows."""
    # ========== GIVEN ==========
    not_parquet = tmp_path / "not_parquet.parquet"
    not_parquet.write_text("this is not a Parquet file\n")
    schema = f"PARQUET_FAILED_{uuid.uuid4().hex[:8]}"
    try:
        table = f"{schema}.parquet_rows"
        reusable_live_deployment.connect(
            "--command",
            f"CREATE SCHEMA {schema}; CREATE TABLE {table} (old_column VARCHAR(10)); "  # noqa: S608 - test-generated identifier
            f"INSERT INTO {table} VALUES ('kept');",
            capture_output=True,
        )

        statement = _inferred_table_statement(table, not_parquet, replace=True)

        # ========== WHEN ==========
        with pytest.raises(subprocess.CalledProcessError):
            reusable_live_deployment.connect(
                "--command", statement, capture_output=True
            )

        # ========== THEN ==========
        proc = reusable_live_deployment.connect(
            "--command",
            f"SELECT old_column FROM {table};",  # noqa: S608 - test-generated identifier
            capture_output=True,
        )
        assert "kept" in proc.stdout
        tables = reusable_live_deployment.connect(
            "--csv",
            "--command",
            "SELECT TABLE_NAME FROM SYS.EXA_ALL_TABLES "  # noqa: S608 - test-generated identifier
            f"WHERE TABLE_SCHEMA = '{schema.upper()}' ORDER BY TABLE_NAME;",
            capture_output=True,
        )
        assert list(csv.reader(tables.stdout.splitlines()))[1:] == [["PARQUET_ROWS"]]
    finally:
        _drop_schema(reusable_live_deployment, schema)


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
@pytest.mark.openspec("local-parquet-import")
def test_user_without_schema_ownership_creates_and_replaces_tables(
    reusable_live_deployment: Deployment,
    tmp_path: Path,
) -> None:
    """A non-owner with ANY TABLE privileges creates and replaces tables."""
    # ========== GIVEN ==========
    local_only = _client_only_parquet(tmp_path)
    suffix = uuid.uuid4().hex[:8].upper()
    schema = f"PARQUET_FOREIGN_{suffix}"
    user = f"PARQUET_USER_{suffix}"
    password = f"Parquet{suffix}"
    try:
        reusable_live_deployment.connect(
            "--command",
            f"CREATE SCHEMA {schema}; "  # noqa: S608 - test-generated identifier
            f"CREATE TABLE {schema}.parquet_rows (old_column VARCHAR(10)); "
            f"INSERT INTO {schema}.parquet_rows VALUES ('stale'); "
            f'CREATE USER {user} IDENTIFIED BY "{password}"; '
            "GRANT CREATE SESSION, CREATE ANY TABLE, DROP ANY TABLE, SELECT ANY TABLE, "
            f"INSERT ANY TABLE, IMPORT TO {user};",
            capture_output=True,
        )
        statements = _inferred_table_statement(
            f"{schema}.new_rows", local_only
        ) + _inferred_table_statement(
            f"{schema}.parquet_rows", local_only, replace=True
        )

        # ========== WHEN ==========
        reusable_live_deployment.connect(
            "-u", user, "-p", password, "--command", statements, capture_output=True
        )

        # ========== THEN ==========
        for table in ("new_rows", "parquet_rows"):
            names = reusable_live_deployment.connect(
                "--csv",
                "--command",
                f'SELECT "name" FROM {schema}.{table} ORDER BY "id";',  # noqa: S608 - test-generated identifier
                capture_output=True,
            )
            assert list(csv.reader(names.stdout.splitlines()))[1:] == [
                ["alpha"],
                ["beta"],
                ["gamma"],
            ]
        tables = reusable_live_deployment.connect(
            "--csv",
            "--command",
            "SELECT TABLE_NAME FROM SYS.EXA_ALL_TABLES "  # noqa: S608 - test-generated identifier
            f"WHERE TABLE_SCHEMA = '{schema}' ORDER BY TABLE_NAME;",
            capture_output=True,
        )
        assert list(csv.reader(tables.stdout.splitlines()))[1:] == [
            ["NEW_ROWS"],
            ["PARQUET_ROWS"],
        ]
    finally:
        _drop_schema(reusable_live_deployment, schema)
        reusable_live_deployment.connect(
            "--command", f"DROP USER IF EXISTS {user};", capture_output=True
        )


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
@pytest.mark.openspec("local-parquet-import")
def test_create_table_from_local_parquet_rejects_unsupported_path(
    shared_live_deployment: Deployment,
    tmp_path: Path,
) -> None:
    """A path the driver cannot import fails with the supported characters named."""
    # ========== GIVEN ==========
    unsupported = tmp_path / "sales(2024).parquet"
    shutil.copy(_PARQUET_FIXTURE, unsupported)

    statement = _inferred_table_statement("s.parquet_rows", unsupported)

    # ========== WHEN ==========
    with pytest.raises(subprocess.CalledProcessError) as error_info:
        shared_live_deployment.connect("--command", statement, capture_output=True)

    # ========== THEN ==========
    error = error_info.value
    combined = _decode_stream(error.stdout) + _decode_stream(error.stderr)
    assert "may only contain ASCII letters" in combined, combined


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
@pytest.mark.openspec("local-parquet-import")
def test_create_table_from_local_parquet_subselect_names_supported_form(
    shared_live_deployment: Deployment,
    tmp_path: Path,
) -> None:
    """An unsupported statement shape fails with the supported form in the error."""
    # ========== GIVEN ==========
    local_only = _client_only_parquet(tmp_path)
    statement = (
        f'CREATE TABLE s.t AS SELECT "id" FROM '  # noqa: S608 - test-generated identifier
        f"(IMPORT FROM LOCAL PARQUET FILE '{local_only}');"
    )

    # ========== WHEN ==========
    with pytest.raises(subprocess.CalledProcessError) as error_info:
        shared_live_deployment.connect("--command", statement, capture_output=True)

    # ========== THEN ==========
    error = error_info.value
    combined = _decode_stream(error.stdout) + _decode_stream(error.stderr)
    assert "CREATE [OR REPLACE] TABLE <schema>.<table> AS" in combined, combined


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
@pytest.mark.stress
@pytest.mark.skipif(
    not os.environ.get(_STRESS_ENV),
    reason=f"Stress test is disabled by default; enable with {_STRESS_ENV}=1",
)
def test_import_large_csv_completes_or_fails_actionably(
    isolated_live_deployment: Deployment,
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
        (
            'CREATE TABLE Users ("id" INT, "user_id" VARCHAR(32), '
            '"firstname" VARCHAR(100), "lastname" VARCHAR(100), '
            '"sex" VARCHAR(10), "email" VARCHAR(255));'
        ),
        f"IMPORT INTO Users FROM LOCAL CSV FILE '{csv_path}';",
        'SELECT COUNT(*) AS "n" FROM Users;',
        "exit",
    ]

    # ========== WHEN ==========
    # The timeout is enforced by the subprocess itself: on expiry the child is
    # killed and TimeoutExpired is raised, so a hanging import cannot block the
    # rest of the test run.
    start = time.monotonic()
    try:
        proc = isolated_live_deployment.connect(
            input="\n".join(statements),
            capture_output=True,
            timeout=timeout_s,
        )
    except subprocess.TimeoutExpired as exc:
        partial = _decode_stream(exc.stdout) + _decode_stream(exc.stderr)
        pytest.fail(
            f"Import hung: killed after {timeout_s}s "
            f"(set {_TIMEOUT_ENV} to adjust the budget). Partial output: {partial!r}"
        )
    except subprocess.CalledProcessError as exc:
        # The launcher runs with check=True, so a failing import lands here
        # instead of returning a non-zero code.
        failed = True
        combined = (_decode_stream(exc.stdout) + _decode_stream(exc.stderr)).lower()
    else:
        combined = (proc.stdout + proc.stderr).lower()
        failed = "error" in combined
    elapsed = time.monotonic() - start
    logging.info("Import of %s MB finished in %.0fs", size_mb, elapsed)

    # ========== THEN ==========
    if failed:
        # Failure must include actionable size/timeout guidance, not a silent hang
        assert any(
            token in combined
            for token in ("timeout", "size", "memory", "disk", "too large", "limit")
        ), f"Stress failure was not actionable: {combined!r}"


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
@pytest.mark.openspec("local-parquet-import")
def test_import_parquet_missing_file_leaves_target_table_unchanged(
    shared_live_deployment: Deployment,
    tmp_path: Path,
) -> None:
    """A failed LOCAL PARQUET import neither empties nor partially fills the table."""
    # Given a table already holding the fixture's rows
    schema = f"test_import_parquet_keep_{uuid.uuid4().hex[:8]}"
    fixture = _PARQUET_FIXTURE.absolute()
    shared_live_deployment.connect(
        "--command",
        "\n".join(
            [
                *_parquet_rows_statements(schema),
                f"IMPORT INTO ParquetRows FROM LOCAL PARQUET FILE '{fixture}';",
            ]
        ),
        capture_output=True,
    )
    missing = tmp_path / "does_not_exist.parquet"

    # When an import from a missing file into that table fails
    with pytest.raises(subprocess.CalledProcessError):
        shared_live_deployment.connect(
            "--command",
            f"IMPORT INTO {schema}.ParquetRows FROM LOCAL PARQUET FILE '{missing}';",
            capture_output=True,
        )

    # Then the table still holds exactly its original rows
    proc = shared_live_deployment.connect(
        "--csv",
        "--command",
        f'SELECT "name" FROM {schema}.ParquetRows ORDER BY "id";',  # noqa: S608
        capture_output=True,
    )
    assert proc.stdout.split() == ["name", "alpha", "beta", "gamma"]
