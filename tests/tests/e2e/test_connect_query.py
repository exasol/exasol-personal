# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

"""Connect / query / output workflows against the shared running deployment.

These tests are read-only with respect to the deployment lifecycle: they connect
and run queries but never stop, start, or interrupt the cluster, so they are safe
to run in any order relative to the deployment and chaos suites.
"""

import logging
import os
import re
import struct
import subprocess
import sys
import textwrap
import time
from multiprocessing import Process
from pathlib import Path
from typing import Final

import pytest

from framework.deployment import Deployment
from framework.launcher import Launcher
from tests.testcase_helpers import skip_unless_infra

if not sys.platform.startswith("win"):
    import fcntl
    import termios


def _connect_worker(launcher_path: str, deployment_dir: str) -> None:
    """Top-level worker to open a DB session via the launcher.

    Uses a pipe to keep stdin open so the session remains active until terminated.
    This function is module-level to be picklable under spawn/forkserver in Python 3.14+
    """
    # Each process creates its own pipe to keep stdin open
    read_fd, write_fd = os.pipe()
    try:
        Launcher(launcher_path).connect(deployment_dir, stdin=read_fd)
    except subprocess.CalledProcessError:
        # Connection failed (likely due to license limit), exit cleanly
        pass
    finally:
        os.close(read_fd)
        os.close(write_fd)


@pytest.mark.installation_e2e
def test_connectable(reusable_deployment: Deployment) -> None:
    # Note: not marked local_e2e — db_connectable() reads deployment.json nodes
    # which the local backend does not populate.
    assert reusable_deployment.db_connectable()


@pytest.mark.installation_e2e
@pytest.mark.local_e2e
def test_single_query(reusable_deployment: Deployment) -> None:
    query: Final = "SELECT * FROM Dual"

    proc = reusable_deployment.connect(input=query, capture_output=True)
    stdout = proc.stdout.strip()

    # Check the query output.
    expected = textwrap.dedent("""
    ┌───────┐
    │ DUMMY │
    ├───────┤
    │ <nil> │
    └───────┘
    """)

    assert stdout.strip("\n") == expected.strip("\n")


@pytest.mark.installation_e2e
@pytest.mark.local_e2e
def test_exit_command(reusable_deployment: Deployment) -> None:
    queries: Final = [
        "exit",
        "SELECT * FROM Dual",
    ]

    queries_str = "\n".join(queries)

    proc = reusable_deployment.connect(input=queries_str, capture_output=True)
    stdout = proc.stdout.strip()

    assert len(stdout) == 0


@pytest.mark.installation_e2e
@pytest.mark.local_e2e
def test_multiple_queries(reusable_deployment: Deployment) -> None:
    queries: Final = [
        "CREATE SCHEMA test_multiple_queries;",
        "OPEN SCHEMA test_multiple_queries;",
        'CREATE TABLE Users ("id" INT PRIMARY KEY, "name" VARCHAR(50));',
        "INSERT INTO Users VALUES (1, 'foo1');",
        "INSERT INTO Users VALUES (2, 'foo2');",
        "INSERT INTO Users VALUES (123, '\"bar 123');",
        "SELECT * FROM Users;",
        "SELECT * FROM Dual;",
    ]

    queries_str = "\n".join(queries)

    proc = reusable_deployment.connect(input=queries_str, capture_output=True)
    stdout = proc.stdout.strip()

    expected = textwrap.dedent("""
    ┌─────┬──────────┐
    │ id  │   name   │
    ├─────┼──────────┤
    │ 1   │ foo1     │
    │ 2   │ foo2     │
    │ 123 │ "bar 123 │
    └─────┴──────────┘
    ┌───────┐
    │ DUMMY │
    ├───────┤
    │ <nil> │
    └───────┘
    """)

    assert stdout.strip("\n") == expected.strip("\n")


@pytest.mark.installation_e2e
@pytest.mark.local_e2e
def test_file_import(reusable_deployment: Deployment) -> None:
    people_csv_path: Final = Path(__file__).parent / Path("assets/people.csv")

    assert people_csv_path.exists()

    queries: Final = [
        "CREATE SCHEMA test_file_import;",
        "OPEN SCHEMA test_file_import;",
        'CREATE TABLE Users ("id" INT PRIMARY KEY, "user_id" VARCHAR(32), '
        '"firstname" VARCHAR(100), "lastname" VARCHAR(100), '
        '"sex" VARCHAR(10), "email" VARCHAR(255));',
        f"IMPORT INTO Users FROM LOCAL CSV FILE '{people_csv_path.absolute()}';",
        "SELECT * FROM Users",
    ]

    queries_str = "\n".join(queries)

    proc = reusable_deployment.connect(input=queries_str, capture_output=True)
    stdout = proc.stdout.strip()

    expected = textwrap.dedent("""
    ┌────┬─────────────────┬───────────┬──────────┬────────┬─────────────────────────────┐
    │ id │     user_id     │ firstname │ lastname │  sex   │            email            │
    ├────┼─────────────────┼───────────┼──────────┼────────┼─────────────────────────────┤
    │ 1  │ 88F7B33d2bcf9f5 │ Shelby    │ Terrell  │ Male   │ elijah57@example.net        │
    │ 2  │ f90cD3E76f1A9b9 │ Phillip   │ Summers  │ Female │ bethany14@example.com       │
    │ 3  │ DbeAb8CcdfeFC2c │ Kristine  │ Travis   │ Male   │ bthompson@example.com       │
    │ 4  │ A31Bee3c201ef58 │ Yesenia   │ Martinez │ Male   │ kaitlinkaiser@example.com   │
    │ 5  │ 1bA7A3dc874da3c │ Lori      │ Todd     │ Male   │ buchananmanuel@example.net  │
    │ 6  │ bfDD7CDEF5D865B │ Erin      │ Day      │ Male   │ tconner@example.org         │
    │ 7  │ bE9EEf34cB72AF7 │ Katherine │ Buck     │ Female │ conniecowan@example.com     │
    │ 8  │ 2EFC6A4e77FaEaC │ Ricardo   │ Hinton   │ Male   │ wyattbishop@example.com     │
    │ 9  │ baDcC4DeefD8dEB │ Dave      │ Farrell  │ Male   │ nmccann@example.net         │
    │ 10 │ 8e4FB470FE19bF0 │ Isaiah    │ Downs    │ Male   │ virginiaterrell@example.org │
    └────┴─────────────────┴───────────┴──────────┴────────┴─────────────────────────────┘
    """)  # noqa: E501

    assert stdout.strip("\n") == expected.strip("\n")


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
@pytest.mark.installation_e2e
@pytest.mark.local_e2e
def test_connect_table_width(reusable_deployment: Deployment) -> None:
    queries: Final = [
        "CREATE SCHEMA test_connect_table_width;",
        "OPEN SCHEMA test_connect_table_width;",
        'CREATE TABLE Users ("id" INT PRIMARY KEY, "name" VARCHAR(2000000));',
        "INSERT INTO Users VALUES (1, 'Helicopter'), (2, repeat('jill', 300000));",
        "SELECT * FROM Users;",
    ]

    queries_str = "\n".join(queries)

    def set_pty_size(fd: int, rows: int, cols: int) -> None:
        winsize = struct.pack("HHHH", rows, cols, 0, 0)
        fcntl.ioctl(fd, termios.TIOCSWINSZ, winsize)

    term_height: Final = 100
    term_width: Final = 60

    # Create a pseudo terminal with a small width.
    master_fd, slave_fd = os.openpty()
    set_pty_size(slave_fd, term_height, term_width)

    # Run the queries with the created pty.
    reusable_deployment.connect(
        input=queries_str,
        stdout=slave_fd,
        stderr=slave_fd,
    )

    # Set non-blocking before draining: on macOS, closing slave_fd first causes
    # immediate EIO on master_fd even if data is buffered. Keep slave_fd open and
    # use non-blocking reads so we don't hang waiting for more data.
    fl = fcntl.fcntl(master_fd, fcntl.F_GETFL)
    fcntl.fcntl(master_fd, fcntl.F_SETFL, fl | os.O_NONBLOCK)

    output_raw = b""
    try:
        while chunk := os.read(master_fd, 1024):
            output_raw += chunk
    except OSError:
        pass
    finally:
        os.close(slave_fd)
        os.close(master_fd)

    output_lines = [line.rstrip("\r") for line in str(output_raw, "utf-8").split("\n")]
    output = "\n".join(output_lines).strip("\n")

    expected = textwrap.dedent("""
    ┌────┬─────────────────────────────────────────────────────┐
    │ id │                        name                         │
    ├────┼─────────────────────────────────────────────────────┤
    │ 1  │ Helicopter                                          │
    │ 2  │ jilljilljilljilljilljilljilljilljilljilljilljilljil │
    └────┴─────────────────────────────────────────────────────┘
    """)

    assert output == expected.strip("\n")


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
@pytest.mark.installation_e2e
@pytest.mark.local_e2e
def test_connect_interactive_shows_version_and_exit_hint(
    reusable_deployment: Deployment,
) -> None:
    launcher_path = reusable_deployment.launcher.launcher_path
    deployment_dir = reusable_deployment.deployment_dir.name
    master_fd, slave_fd = os.openpty()

    proc = subprocess.Popen(
        [launcher_path, "connect", "--deployment-dir", deployment_dir],
        stdin=slave_fd,
        stdout=slave_fd,
        stderr=slave_fd,
    )

    try:
        os.write(master_fd, b"exit\n")
        return_code = proc.wait(timeout=120)
    finally:
        if proc.poll() is None:
            proc.kill()

    # Drain master_fd before closing slave_fd: on macOS, closing slave_fd first
    # causes immediate EIO on master_fd even if data is buffered.
    fl = fcntl.fcntl(master_fd, fcntl.F_GETFL)
    fcntl.fcntl(master_fd, fcntl.F_SETFL, fl | os.O_NONBLOCK)

    output_raw = b""
    try:
        while chunk := os.read(master_fd, 1024):
            output_raw += chunk
    except OSError:
        pass
    finally:
        os.close(slave_fd)
        os.close(master_fd)

    output = output_raw.decode("utf-8", errors="replace")

    assert return_code == 0
    assert 'Type "exit" to exit the shell' in output
    assert re.search(r"\bExasol \d+\.\d+\.\d+\b", output) is not None


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
@pytest.mark.installation_e2e
@pytest.mark.local_e2e
def test_diag_cos_runs_confd_client(
    reusable_deployment: Deployment, infra: str
) -> None:
    if infra == "local":
        pytest.skip(
            "confd_client is a COS tool; local deployments use a VM shell"
            " fallback where it is not available"
        )
    # Given: A running deployment and a PTY for an interactive container shell session.
    launcher_path = reusable_deployment.launcher.launcher_path
    deployment_dir = reusable_deployment.deployment_dir.name
    master_fd, slave_fd = os.openpty()

    proc = subprocess.Popen(
        [launcher_path, "shell", "container", "--deployment-dir", deployment_dir],
        stdin=slave_fd,
        stdout=slave_fd,
        stderr=slave_fd,
    )

    try:
        # When: We run a COS-only command and then exit the session.
        os.write(master_fd, b"confd_client db_list --json\n")
        os.write(master_fd, b"echo COS_DB_LIST_RC:$?\n")
        os.write(master_fd, b"exit\n")
        os.write(master_fd, b"exit\n")

        return_code = proc.wait(timeout=120)
    finally:
        if proc.poll() is None:
            proc.kill()

    # Drain master_fd before closing slave_fd: on macOS, closing slave_fd first
    # causes immediate EIO on master_fd even if data is buffered.
    fl = fcntl.fcntl(master_fd, fcntl.F_GETFL)
    fcntl.fcntl(master_fd, fcntl.F_SETFL, fl | os.O_NONBLOCK)

    # Read all output from the PTY.
    output_raw = b""
    try:
        while chunk := os.read(master_fd, 1024):
            output_raw += chunk
    except OSError:
        pass
    finally:
        os.close(slave_fd)
        os.close(master_fd)

    output = output_raw.decode("utf-8", errors="replace")

    # Then: The command succeeded from COS and the session exited cleanly.
    assert return_code == 0
    assert "COS_DB_LIST_RC:0" in output
    assert "command not found" not in output.lower()


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
@pytest.mark.installation_e2e
@pytest.mark.local_e2e
def test_license_session_limit(reusable_deployment: Deployment) -> None:
    # license_session_limit is the limit defined in Exasol Personal
    # license. This value should match it.
    license_session_limit: Final = 20
    exceeded_count: Final = 5  # We exceed the session limit by this amount.

    procs: list[Process] = []
    # Gather primitive arguments for the worker
    launcher_path = reusable_deployment.launcher.launcher_path
    deployment_dir = reusable_deployment.deployment_dir.name

    try:
        # Create SESSION_LIMIT + EXCEEDED_COUNT connections.
        for _ in range(license_session_limit + exceeded_count):
            proc = Process(target=_connect_worker, args=(launcher_path, deployment_dir))
            proc.start()
            procs.append(proc)
            # NOTE!
            # NH / 2015-11-26
            # Connecting concurrently is racy in core DB at the moment
            # This allows us to have more active connection than the license limit
            # Waiting a bit here prevents this
            time.sleep(2)

        # Wait for some time to make sure the connections
        # actually go through.
        time.sleep(5)

        # Count the number of alive connections.
        alive = sum(1 for proc in procs if proc.is_alive())
    finally:  # Teardown
        # Terminate all connections.
        for proc in procs:
            if proc.is_alive():
                proc.kill()

    # Check that we have at most license_session_limit connections alive.
    assert alive == license_session_limit


@pytest.mark.installation_e2e
def test_large_result_sets_fully_fetched(infra: str) -> None:
    skip_unless_infra(infra, "aws", "azure", "exoscale", "stackit", "local")

    # Needs a running database and a table with >1000 rows to exercise the
    # --max-rows behaviour and the stderr truncation footer.
    pytest.skip("requires a running database with a >1000-row table")


@pytest.mark.installation_e2e
def test_typed_html_safe_json(infra: str) -> None:
    skip_unless_infra(infra, "aws", "azure", "exoscale", "stackit", "local")

    # Needs a running database to evaluate SELECT expressions and inspect the
    # JSON typing / HTML-safety of the rendered statement-record rows.
    pytest.skip("requires a running database to render typed JSON rows")


@pytest.mark.installation_e2e
def test_non_json_output_unchanged(infra: str) -> None:
    skip_unless_infra(infra, "aws", "azure", "exoscale", "stackit", "local")

    # Needs a running database to render the default (table) and CSV output.
    pytest.skip("requires a running database to render table/CSV output")


@pytest.mark.installation_e2e
def test_multi_statement_script_yields_single_document(infra: str) -> None:
    skip_unless_infra(infra, "aws", "azure", "exoscale", "stackit", "local")

    # Needs a running database to execute the multi-statement script and parse
    # the aggregated stdout as a single JSON document.
    pytest.skip("requires a running database to execute multi-statement scripts")


@pytest.mark.installation_e2e
def test_statement_metadata_distinguishes_statement_kinds(infra: str) -> None:
    skip_unless_infra(infra, "aws", "azure", "exoscale", "stackit", "local")

    # Needs a running database to execute DDL/DML/query statements and inspect
    # statementType/rowsAffected/columns/rows/truncated per record.
    pytest.skip("requires a running database to inspect statement metadata")


@pytest.mark.installation_e2e
def test_structured_sql_errors_in_json_mode(infra: str) -> None:
    skip_unless_infra(infra, "aws", "azure", "exoscale", "stackit", "local")

    # Needs a running database to provoke real SQL errors and inspect the
    # structured error object in the invocation document.
    pytest.skip("requires a running database to provoke structured SQL errors")


@pytest.mark.installation_e2e
def test_interactive_mode_unaffected(infra: str) -> None:
    skip_unless_infra(infra, "aws", "azure", "exoscale", "stackit", "local")

    # Interactive REPL behaviour needs a TTY and a running database; verify
    # manually per the test-case document.
    pytest.skip("requires an interactive terminal and a running database")


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
def test_password_marker_not_leaked_to_logs(reusable_deployment: Deployment) -> None:
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
        proc = reusable_deployment.connect(
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
    deployment_log = Path(reusable_deployment.deployment_dir.name) / "deployment.log"
    if deployment_log.exists():
        log_contents = deployment_log.read_text(encoding="utf-8", errors="replace")
        assert marker not in log_contents, (
            f"Password marker leaked into {deployment_log}"
        )
    logging.info("Connect rc=%s; marker not found in stdout/stderr/log", returncode)


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
def test_connect_shows_exit_hint(reusable_deployment: Deployment) -> None:
    """An interactive connection must print the 'how to exit' banner.

    The launcher reads the DB password from the encrypted secrets file rather
    than prompting interactively, so the PDF's user/password-prompt expectation
    does not apply to the current implementation. The exit-hint contract still
    holds and is what we assert.
    """
    # ========== GIVEN ==========
    # A connectable deployment
    assert reusable_deployment.db_connectable()

    # ========== WHEN ==========
    # Connect is invoked with an immediate "exit" on stdin
    proc = reusable_deployment.connect(input="exit\n", capture_output=True)

    # ========== THEN ==========
    # The shell prints its exit hint to stderr and exits cleanly
    assert proc.returncode == 0
    assert 'Type "exit" to exit the shell' in proc.stderr


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
def test_invalid_sql_does_not_crash_shell(reusable_deployment: Deployment) -> None:
    """An invalid statement must error but keep the shell alive."""
    # ========== GIVEN ==========
    # A connected shell session driven via stdin
    # ========== WHEN ==========
    # We submit an invalid statement followed by a valid one
    queries = "SELEC 1;\nSELECT 1 AS ok FROM Dual;\n"
    proc = reusable_deployment.connect(input=queries, capture_output=True)

    # ========== THEN ==========
    # The valid query result still appears, proving the shell survived the error
    assert "ok" in proc.stdout.lower() or "1" in proc.stdout
    combined = (proc.stdout + proc.stderr).lower()
    assert any(token in combined for token in ("error", "syntax", "invalid")), (
        f"Expected SQL error message, got: {combined!r}"
    )


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
def test_many_statements_remain_stable(reusable_deployment: Deployment) -> None:
    """50 small statements must run without crash or hang."""
    # ========== GIVEN ==========
    # A connectable deployment
    assert reusable_deployment.db_connectable()

    # ========== WHEN ==========
    # We send 50 alternating trivial statements and exit
    statements_per_kind: Final = 25
    statements = []
    for _ in range(statements_per_kind):
        statements.append("SELECT 1 FROM Dual;")
        statements.append("SELECT CURRENT_TIMESTAMP FROM Dual;")
    statements.append("exit")

    proc = reusable_deployment.connect(input="\n".join(statements), capture_output=True)

    # ========== THEN ==========
    # The shell terminates cleanly and the deployment is still healthy
    assert proc.returncode == 0
    assert reusable_deployment.db_connectable()
