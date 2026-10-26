# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

import json
import os
import re
import stat
import subprocess
import uuid
from pathlib import Path
from typing import Any

import pytest
import requests

from framework.deployment import Deployment
from framework.outputs import get_outputs

pytestmark = [
    pytest.mark.openspec("bucketfs-credentials"),
    pytest.mark.providers("aws", "azure", "exoscale"),
]


def _credentials(deployment: Deployment) -> dict[str, Any]:
    path = Path(deployment.deployment_dir.name) / "secrets.json"
    if os.name != "nt":
        assert stat.S_IMODE(path.stat().st_mode) == stat.S_IRUSR | stat.S_IWUSR
    data: dict[str, Any] = json.loads(path.read_text())
    return dict(data["bucketfs"]["bfsdefault"]["buckets"]["default"])


def _check_https(deployment: Deployment, credentials: dict[str, Any]) -> None:
    host = get_outputs(deployment.deployment_dir.name).nodes["n11"].publicIp
    url = f"https://{host}:2581/default/bucketfs-test-{uuid.uuid4().hex}.txt"
    write = credentials["write"]
    read = credentials["read"]
    write_auth = (write["username"], write["password"])
    read_auth = (read["username"], read["password"])
    content = b"BucketFS credential and lifecycle evidence\n"
    verify_ssl = False  # Disposable deployments use self-signed certificates.
    try:
        uploaded = requests.put(
            url, data=content, auth=write_auth, verify=verify_ssl, timeout=60
        )
        uploaded.raise_for_status()
        for auth in (read_auth, (read["username"], write["password"])):
            downloaded = requests.get(url, auth=auth, verify=verify_ssl, timeout=60)
            downloaded.raise_for_status()
            assert downloaded.content == content
        assert requests.get(url, verify=verify_ssl, timeout=60).status_code in (
            401,
            403,
        )
        assert requests.get(
            url, auth=("r", "wrong-password"), verify=verify_ssl, timeout=60
        ).status_code in (401, 403)
    finally:
        deleted = requests.delete(url, auth=write_auth, verify=verify_ssl, timeout=60)
        deleted.raise_for_status()


def test_private_bucket_credentials_survive_stop_start(
    isolated_live_deployment: Deployment,
) -> None:
    deployment = isolated_live_deployment
    credentials = _credentials(deployment)
    for identity, username in (("read", "r"), ("write", "w")):
        if credentials[identity]["username"] != username or not re.fullmatch(
            r"[A-Za-z0-9]{32,}",
            credentials[identity]["password"],
        ):
            pytest.fail("Default BucketFS credential has an unexpected format")
    if credentials["read"]["password"] == credentials["write"]["password"]:
        pytest.fail("BucketFS read and write passwords must differ")

    command = (
        'c4="$HOME/.local/bin/c4"; '
        'play=$("$c4" config -e .play.id) && '
        "printf '%s\n' 'confd_client bucket_modify bucketfs_name: bfsdefault "
        "bucket_name: default public: False' | "
        '"$c4" connect -s cos -i "$play"'
    )
    subprocess.run(
        [
            deployment.launcher.launcher_path,
            "shell",
            "host",
            "--deployment-dir",
            deployment.deployment_dir.name,
            "--",
            "bash",
            "-c",
            command,
        ],
        check=True,
        capture_output=True,
        text=True,
    )
    _check_https(deployment, credentials)
    deployment.stop()
    deployment.start()
    if _credentials(deployment) != credentials:
        pytest.fail("BucketFS credentials changed across stop/start")
    _check_https(deployment, credentials)
    log = (Path(deployment.deployment_dir.name) / "deployment.log").read_text()
    for identity in ("read", "write"):
        if credentials[identity]["password"] in log:
            pytest.fail("BucketFS password was written to deployment.log")
