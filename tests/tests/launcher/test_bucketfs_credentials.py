# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

import base64
import json
import os
import re
import shutil
import subprocess
from pathlib import Path

import pytest

REPO_ROOT = Path(__file__).resolve().parents[3]
SCRIPTS = REPO_ROOT / "assets/installation/ubuntu/files/opt/exasol_launcher/scripts"
pytestmark = pytest.mark.openspec("bucketfs-credentials")


@pytest.mark.parametrize("provider", ["aws", "azure", "exoscale", "stackit"])
def test_cloud_bucketfs_credentials_contract(provider: str) -> None:
    root = REPO_ROOT / "assets/infrastructure" / provider
    secrets = (root / "secrets.tf").read_text()
    outputs = (root / "outputs.tf").read_text()
    cloudinit = (root / "cloudinit.tf").read_text()
    for identity in ("read", "write"):
        resource = re.search(
            rf'resource "random_password" "bucketfs_{identity}" {{([^}}]+)}}',
            secrets,
        )
        assert resource is not None
        assert re.search(r"length\s*=\s*32", resource[1])
        assert re.search(r"special\s*=\s*false", resource[1])
        assert "keepers" not in resource[1]
        assert re.search(
            rf"password\s*= random_password.bucketfs_{identity}.result", outputs
        )
        assert f"base64encode(random_password.bucketfs_{identity}.result)" in cloudinit
    assert 'resource "local_sensitive_file" "deployment_secrets"' in outputs
    assert re.search(r'file_permission\s*=\s*"0600"', outputs)
    assert re.search(
        r"path\s*= local.infrastructure_json_target_path\s+"
        r"(?:#[^\n]*\n\s*)*defer\s*= true\s+"
        r'owner\s*= "ubuntu:ubuntu"\s+permissions\s*= "0600"',
        cloudinit,
    )
    service = (
        REPO_ROOT
        / "assets/installation/ubuntu/files/etc/systemd/system"
        / "exasol_launcher_post_install.service"
    ).read_text()
    assert service.index("configureBucketFS.sh") < service.index(
        "post_install.complete", service.index("ExecStart=")
    )


@pytest.mark.skipif(os.name == "nt", reason="Ubuntu installer requires bash")
@pytest.mark.parametrize("public", [True, False])
@pytest.mark.parametrize("failure", ["none", "info", "modify", "config", "missing"])
def test_bucketfs_installer_preserves_policy_and_hides_secrets(
    tmp_path: Path,
    *,
    public: bool,
    failure: str,
) -> None:
    for name in ("configureBucketFS.sh", "logging.sh"):
        shutil.copyfile(SCRIPTS / name, tmp_path / name)
    (tmp_path / "config.sh").write_text('infra_jq() { jq "$@" "$TEST_INFRA"; }\n')
    (tmp_path / "shared_post_install.sh").write_text('C4_PATH="$TEST_BIN/c4"\n')
    binary = tmp_path / "bin"
    binary.mkdir()
    read_password = "Read" * 8
    write_password = "Write" * 8
    infra = tmp_path / "infrastructure.json"
    infra.write_text(
        json.dumps(
            {
                "bucketfsReadPasswordB64": base64.b64encode(
                    read_password.encode()
                ).decode(),
                "bucketfsWritePasswordB64": base64.b64encode(
                    write_password.encode()
                ).decode(),
            }
            if failure != "missing"
            else {}
        )
    )
    state = tmp_path / "state.json"
    state.write_text(json.dumps({"public": public, "updates": []}))
    c4 = binary / "c4"
    c4.write_text("""#!/usr/bin/env python3
import os, subprocess, sys
if sys.argv[1] == "config":
    if os.environ["TEST_FAILURE"] == "config":
        print("PRIVATE-CONFD-RESPONSE", file=sys.stderr)
        sys.exit(1)
    print("play-id")
else:
    script = sys.stdin.read()
    print(script)
    print("PRIVATE-CONFD-RESPONSE", file=sys.stderr)
    sys.exit(subprocess.run(["bash"], input=script, text=True).returncode)
""")
    confd = binary / "confd_client"
    confd.write_text("""#!/usr/bin/env python3
import json, os, pathlib, sys
assert sys.argv[1] == "--json" and sys.argv[3:] == ["-a", "-"]
params = json.load(sys.stdin)
assert params["bucketfs_name"] == "bfsdefault"
path = pathlib.Path(os.environ["TEST_STATE"])
state = json.loads(path.read_text())
command = sys.argv[2]
operation = command.removeprefix("bucketfs_").removeprefix("bucket_")
if os.environ["TEST_FAILURE"] == operation:
    print("PRIVATE-CONFD-RESPONSE", file=sys.stderr)
    sys.exit(1)
if command == "bucketfs_info":
    bucket = dict(state, read_passwd="PRIVATE-CONFD-RESPONSE")
    print(json.dumps({"buckets": {"default": bucket}}))
else:
    assert command == "bucket_modify" and params["bucket_name"] == "default"
    assert params["public"] == state["public"]
    state["updates"].append(params)
    path.write_text(json.dumps(state))
    print("PRIVATE-CONFD-RESPONSE", file=sys.stderr)
""")
    c4.chmod(0o700)
    confd.chmod(0o700)
    environment = dict(
        os.environ,
        TEST_BIN=str(binary),
        TEST_INFRA=str(infra),
        TEST_STATE=str(state),
        TEST_FAILURE=failure,
        PATH=f"{binary}{os.pathsep}{os.environ['PATH']}",
    )
    result = subprocess.run(
        [shutil.which("bash") or "bash", "-x", str(tmp_path / "configureBucketFS.sh")],
        env=environment,
        capture_output=True,
        text=True,
        check=False,
    )
    assert (result.returncode == 0) == (failure == "none")
    if failure != "none":
        assert (
            "Failed to configure BucketFS credentials for bfsdefault/default"
            in result.stdout + result.stderr
        )
    environment["TEST_FAILURE"] = "none"
    if failure != "missing":
        retry = subprocess.run(
            [
                shutil.which("bash") or "bash",
                "-x",
                str(tmp_path / "configureBucketFS.sh"),
            ],
            env=environment,
            capture_output=True,
            text=True,
            check=False,
        )
        assert retry.returncode == 0
        result_output = result.stdout + result.stderr + retry.stdout + retry.stderr
        updates = json.loads(state.read_text())["updates"]
        assert len(updates) == (2 if failure == "none" else 1)
        for update in updates:
            assert update["read_password"] == read_password
            assert update["write_password"] == write_password
            assert update["public"] == public
    else:
        result_output = result.stdout + result.stderr
        assert json.loads(state.read_text())["updates"] == []
    for secret in (read_password, write_password, "PRIVATE-CONFD-RESPONSE"):
        assert secret not in result_output
    for encoded in json.loads(infra.read_text()).values():
        assert encoded not in result_output
