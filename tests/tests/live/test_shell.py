# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

"""Interactive shell behavior against a running cloud deployment."""

import os
import subprocess
import sys

import pytest

from framework.deployment import Deployment
from tests.testcase_helpers import requires_posix_pty

if not sys.platform.startswith("win"):
    import fcntl


@pytest.mark.providers("aws", "azure", "exoscale")
@requires_posix_pty
def test_container_shell_runs_confd_client(
    shared_live_deployment: Deployment,
) -> None:
    launcher_path = shared_live_deployment.launcher.launcher_path
    deployment_dir = shared_live_deployment.deployment_dir.name
    master_fd, slave_fd = os.openpty()

    proc = subprocess.Popen(
        [launcher_path, "shell", "container", "--deployment-dir", deployment_dir],
        stdin=slave_fd,
        stdout=slave_fd,
        stderr=slave_fd,
    )

    try:
        os.write(master_fd, b"confd_client db_list --json\n")
        os.write(master_fd, b"echo COS_DB_LIST_RC:$?\n")
        os.write(master_fd, b"exit\n")
        os.write(master_fd, b"exit\n")
        return_code = proc.wait(timeout=120)
    finally:
        if proc.poll() is None:
            proc.kill()

    flags = fcntl.fcntl(master_fd, fcntl.F_GETFL)
    fcntl.fcntl(master_fd, fcntl.F_SETFL, flags | os.O_NONBLOCK)

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
    assert "COS_DB_LIST_RC:0" in output
    assert "command not found" not in output.lower()
