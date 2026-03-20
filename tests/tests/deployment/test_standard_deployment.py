# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

"""Tests using the standard reusable standard deployment."""

import logging
import os
import platform
import signal
import struct
import subprocess
import sys

if not sys.platform.startswith("win"):
    import fcntl
    import termios
import textwrap
import time
from multiprocessing import Process
from pathlib import Path
from typing import Final

import pytest
import requests
import semver

from framework.deployment import (
    Deployment,
    StatusDatabaseReady,
    StatusInterrupted,
    StatusOperationInProgress,
    StatusStopped,
)
from framework.launcher import DeploymentConfig, Launcher


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


@pytest.fixture(scope="session")
def reusable_deployment(exasol_path: str, infra: str) -> Deployment:  # type: ignore[misc]
    cluster_size = 2 if infra == "aws" else 1
    config = DeploymentConfig(infra=infra, cluster_size=cluster_size)
    deployment = Deployment(Launcher(exasol_path), config=config)
    try:
        deployment_proc = deployment.deploy_no_block()

        # Sleep after Popen to allow the child process to start
        # and update status (needed for Windows).
        if platform.system() == "Windows":
            time.sleep(3)

        logging.info("Check status deployment in progress")
        timeout = 10
        start_time = time.time()
        while True:
            if deployment.has_status(StatusOperationInProgress):
                break

            if time.time() - start_time > timeout:
                logging.info("Timeout expired. Status incorrect")
                msg = f"Expected status `{StatusOperationInProgress}` after `deploy`"
                raise RuntimeError(msg)

            logging.info("Status incorrect. Retrying in 5 seconds")
            time.sleep(5)

        logging.info("Waiting for deploy to complete")

        deploy_return_code = deployment_proc.wait(timeout=20 * 60)
        if deploy_return_code != 0:
            msg = (
                f"Deploy command failed with code {deploy_return_code}\n"
                f"deployment.log tail:\n{deployment.deployment_log_tail()}"
            )
            raise RuntimeError(msg)

        logging.info("Checking status database available")

        if not deployment.has_status(StatusDatabaseReady):
            msg = f"Expected status `{StatusDatabaseReady}` after `deploy`"
            raise RuntimeError(msg)

        yield deployment

    finally:
        deployment.cleanup()


@pytest.mark.installation_e2e
def test_connectable(reusable_deployment: Deployment) -> None:
    assert reusable_deployment.db_connectable()


# All DB-related tests are skipped on Windows, because the SQL shell
# does not output query results, only displaying an empty prompt.
# On Linux, the expected result is shown.


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
@pytest.mark.installation_e2e
def test_single_query(reusable_deployment: Deployment) -> None:
    query: Final = "SELECT * FROM Dual"

    proc = reusable_deployment.connect(input=query, capture_output=True)
    stderr = proc.stderr.strip()
    stdout = proc.stdout.strip()

    lines = stderr.splitlines()
    version_line, exit_hint_lint = lines[0], lines[1]

    assert exit_hint_lint.strip() == 'Type "exit" to exit the shell'

    # Check the Exasol version that is printed as the first line
    exasol_name, exasol_version = version_line.split(" ")

    assert exasol_name == "Exasol"
    assert semver.VersionInfo.is_valid(exasol_version)

    # Check the query output.
    expected = textwrap.dedent("""
    ┌───────┐
    │ DUMMY │
    ├───────┤
    │ <nil> │
    └───────┘
    """)

    assert stdout.strip("\n") == expected.strip("\n")


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
@pytest.mark.installation_e2e
def test_exit_command(reusable_deployment: Deployment) -> None:
    queries: Final = [
        "exit",
        "SELECT * FROM Dual",
    ]

    queries_str = "\n".join(queries)

    proc = reusable_deployment.connect(input=queries_str, capture_output=True)
    stdout = proc.stdout.strip()

    assert len(stdout) == 0


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
@pytest.mark.installation_e2e
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


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
@pytest.mark.installation_e2e
def test_file_import(reusable_deployment: Deployment) -> None:
    people_csv_path: Final = Path(__file__).parent / Path("assets/people.csv")

    assert people_csv_path.exists()

    queries: Final = [
        "CREATE SCHEMA test_file_import;",
        "OPEN SCHEMA test_file_import;",
        'CREATE TABLE Users ("id" INT PRIMARY KEY, "user_id" VARCHAR(32), '
        '"firstname" VARCHAR(100), "lastname" VARCHAR(100), '
        '"sex" VARCHAR(10), "email" VARCHAR(255))',
        f"IMPORT INTO Users FROM LOCAL CSV FILE '{people_csv_path.absolute()}'",
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

    # Read the stdout of the connect command, KiB at a time.
    os.close(slave_fd)
    output_raw = b""
    try:
        while chunk := os.read(master_fd, 1024):
            output_raw += chunk
    except OSError:
        pass

    os.close(master_fd)

    # Skip first line because it contains the DB version
    # which is dynamic.
    actual_lines = str(output_raw, "utf-8").split("\n")[1:]
    actual_lines = [line.rstrip("\r") for line in actual_lines]
    actual = "\n".join(actual_lines).strip("\n")

    expected = textwrap.dedent("""
    Type "exit" to exit the shell
    ┌────┬─────────────────────────────────────────────────────┐
    │ id │                        name                         │
    ├────┼─────────────────────────────────────────────────────┤
    │ 1  │ Helicopter                                          │
    │ 2  │ jilljilljilljilljilljilljilljilljilljilljilljilljil │
    └────┴─────────────────────────────────────────────────────┘
    """)

    assert actual == expected.strip("\n")


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
@pytest.mark.installation_e2e
def test_diag_cos_runs_confd_client(reusable_deployment: Deployment) -> None:
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

        os.close(slave_fd)

    # Read all output from the PTY.
    output_raw = b""
    try:
        while chunk := os.read(master_fd, 1024):
            output_raw += chunk
    except OSError:
        pass
    finally:
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
def test_license_session_limit(reusable_deployment: Deployment) -> None:
    # license_session_limit is the limit defined in Exasol Personal
    # license. This value should match it.
    license_session_limit: Final = 5
    exceeded_count: Final = 3  # We exceed the session limit by this amount.

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


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
@pytest.mark.infrastructure_e2e
def test_stop_and_start(reusable_deployment: Deployment) -> None:
    # Using resuable_deployment fixture
    assert reusable_deployment.db_connectable()

    # Test info before stopping - should show running state
    info_result = reusable_deployment.info()
    assert info_result.returncode == 0
    assert "Exasol Personal" in info_result.stdout
    assert "Cluster Size:" in info_result.stdout
    assert "Cluster State: running" in info_result.stdout

    # Stop the deployment
    stop_result = reusable_deployment.stop()
    assert hasattr(stop_result, "returncode")
    assert stop_result.returncode == 0

    # Test info after stopping - should show stopped state
    info_result = reusable_deployment.info()
    assert info_result.returncode == 0
    assert "Exasol Personal" in info_result.stdout
    assert "Cluster Size:" in info_result.stdout
    assert "Cluster State: stopped" in info_result.stdout

    # Start the deployment again
    start_result = reusable_deployment.start()
    assert hasattr(start_result, "returncode")
    assert start_result.returncode == 0

    # Test info after starting - should show running state again
    info_result = reusable_deployment.info()
    assert info_result.returncode == 0
    assert "Exasol Personal" in info_result.stdout
    assert "Cluster Size:" in info_result.stdout
    assert "Cluster State: running" in info_result.stdout

    # Immediately verify DB is connectable after start completes
    assert reusable_deployment.db_connectable()

    if not sys.platform.startswith("win"):
        connect_result = reusable_deployment.connect()
        assert hasattr(connect_result, "returncode")
        assert connect_result.returncode == 0
    else:
        pytest.skip("Skipping DB connection for Windows OS")


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
@pytest.mark.infrastructure_e2e
def test_start_interrupt_sets_interrupted_state(
    reusable_deployment: Deployment,
) -> None:
    # ========== GIVEN ==========
    # A running deployment
    assert reusable_deployment.db_connectable()

    # ========== WHEN ==========
    # We interrupt a stop operation
    stop_proc = reusable_deployment.stop_no_block()
    time.sleep(2)
    assert reusable_deployment.has_status(StatusOperationInProgress)

    if platform.system() != "Windows":
        os.kill(stop_proc.pid, signal.SIGINT)
    else:
        stop_proc.terminate()

    stop_proc.wait(timeout=30)

    # ========== THEN ==========
    # The deployment status should be interrupted
    assert reusable_deployment.has_status(StatusInterrupted)

    time.sleep(30)  # Allow tofu to release the lock

    # ========== GIVEN ==========
    # A stopped deployment
    assert reusable_deployment.stop().returncode == 0
    assert reusable_deployment.has_status(StatusStopped)

    # ========== WHEN ==========
    # We interrupt a start operation
    start_proc = reusable_deployment.start_no_block()
    time.sleep(2)
    assert reusable_deployment.has_status(StatusOperationInProgress)

    if platform.system() != "Windows":
        os.kill(start_proc.pid, signal.SIGINT)
    else:
        start_proc.terminate()

    start_proc.wait(timeout=30)

    # ========== THEN ==========
    # The deployment status should be interrupted
    assert reusable_deployment.has_status(StatusInterrupted)

    time.sleep(30)  # Allow tofu to release the lock

    # Restore to running state for subsequent tests
    assert reusable_deployment.start().returncode == 0
    assert reusable_deployment.has_status(StatusDatabaseReady)


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
@pytest.mark.infrastructure_e2e
@pytest.mark.provider_aws
def test_remote_archive_registered(
    reusable_deployment: Deployment,
    infra: str,
) -> None:
    """Scenario: Verify that a remote archive volume is registered via Admin UI.

    Given a deployed Exasol cluster with an initialized remote archive volume
    When we authenticate to the Admin UI API using the admin credentials
    And we list the available deployments
    And we query the backup options for the deployment
    Then we should receive a successful response
    And the response should contain the 'default_archive' backup option
    """
    if infra != "aws":
        pytest.skip("Remote archive verification is only supported for AWS today")

    # ========== GIVEN ==========
    # Ensure deployment is running (previous tests may have stopped it)
    if not reusable_deployment.has_status(StatusDatabaseReady):
        start_result = reusable_deployment.start()
        assert start_result.returncode == 0
        assert reusable_deployment.has_status(StatusDatabaseReady)

    # A deployed Exasol cluster with remote archive configured
    host, port = reusable_deployment.admin_ui()
    username, password = reusable_deployment.admin_ui_credentials()
    deployment_id = reusable_deployment.deployment_id()

    base_url = f"https://{host}:{port}/api/v1"
    verify_ssl = False  # equivalent to curl -k (insecure)

    # ========== WHEN ==========
    # We request an access token from the Admin UI API
    token_url = f"{base_url}/token"
    token_payload = {
        "grant_type": "password",
        "username": username,
        "password": password,
    }

    token_response = requests.post(
        token_url,
        data=token_payload,
        headers={"Content-Type": "application/x-www-form-urlencoded"},
        verify=verify_ssl,
        timeout=30,
    )

    token_response.raise_for_status()
    access_token = token_response.json().get("access_token")

    # When we list the available deployments
    deployments_url = f"{base_url}/deployments"

    deployments_response = requests.get(
        deployments_url,
        headers={"Authorization": f"Bearer {access_token}"},
        verify=verify_ssl,
        timeout=30,
    )

    deployments_response.raise_for_status()

    # ========== THEN ==========
    # We should receive a list of deployments
    assert len(deployments_response.json()) > 0, "No deployments found"

    # When we query the backup options for the deployment
    backups_url = f"{base_url}/deployments/{deployment_id}/backups"

    backups_response = requests.options(
        backups_url,
        headers={
            "Accept": "application/json",
            "Authorization": f"Bearer {access_token}",
        },
        verify=verify_ssl,
        timeout=30,
    )

    backups_response.raise_for_status()

    # Then we should receive backup options
    assert len(backups_response.json()) > 0, "No backup options found"

    # And the response should contain the 'default_archive' option
    ok = any(
        backup.get("name") == "default_archive" for backup in backups_response.json()
    )
    assert ok, "Missing default_archive"
