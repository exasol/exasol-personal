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


READ_PASSWORD = "r\"'\\$()\nü"  # noqa: S105
WRITE_PASSWORD = "w\"'\\`$\n!"  # noqa: S105
PRIVATE_RESPONSE = "PRIVATE-CONFD-RESPONSE"


@pytest.fixture
def bucketfs_installer(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> Path:
    if os.name == "nt":
        pytest.skip("Ubuntu installer requires bash")
    for name in ("configureBucketFS.sh", "logging.sh"):
        shutil.copyfile(SCRIPTS / name, tmp_path / name)
    (tmp_path / "config.sh").write_text(
        'infra_jq() { jq "$@" "$TEST_DIR/infra.json"; }\n'
    )
    (tmp_path / "shared_post_install.sh").write_text('C4_PATH="$TEST_DIR/c4"\n')
    (tmp_path / "infra.json").write_text(
        json.dumps(
            {
                "bucketfsReadPasswordB64": base64.b64encode(
                    READ_PASSWORD.encode()
                ).decode(),
                "bucketfsWritePasswordB64": base64.b64encode(
                    WRITE_PASSWORD.encode()
                ).decode(),
            }
        )
    )
    (tmp_path / "info.json").write_text(
        json.dumps(
            {
                "buckets": {
                    "default": {"public": True, "read_passwd": PRIVATE_RESPONSE}
                },
            }
        )
    )
    (tmp_path / "c4").write_text("""#!/usr/bin/env bash
set -euo pipefail
if [[ "$1" == config ]]; then
    [[ "${TEST_FAILURE:-}" != config ]] || exit 1
    echo play-id
else
    [[ "${TEST_FAILURE:-}" != connect ]] || exit 1
    tee /dev/stderr | bash
fi
""")
    (tmp_path / "confd_client").write_text("""#!/usr/bin/env bash
set -euo pipefail
[[ "$1" == --json && "$3" == -a && "$4" == - ]]
request=$(cat)
printf '%s\\n' "$request" >&2
echo PRIVATE-CONFD-RESPONSE >&2
if [[ "$2" == bucketfs_info ]]; then
    cat "$TEST_DIR/info.json"
else
    [[ "$2" == bucket_modify ]]
    printf '%s\\n' "$request" > "$TEST_DIR/request.json"
fi
[[ "${TEST_FAILURE:-}" != "$2" ]]
""")
    for name in ("c4", "confd_client"):
        (tmp_path / name).chmod(0o700)
    monkeypatch.setenv("TEST_DIR", str(tmp_path))
    monkeypatch.setenv("PATH", f"{tmp_path}{os.pathsep}{os.environ['PATH']}")
    return tmp_path


def _run_installer(directory: Path) -> subprocess.CompletedProcess[str]:
    result = subprocess.run(
        [shutil.which("bash") or "bash", "-x", str(directory / "configureBucketFS.sh")],
        capture_output=True,
        text=True,
        check=False,
    )
    output = result.stdout + result.stderr
    for secret in (READ_PASSWORD, WRITE_PASSWORD, PRIVATE_RESPONSE):
        assert secret not in output
        assert base64.b64encode(secret.encode()).decode() not in output
    return result


@pytest.mark.parametrize("public", [True, False])
def test_bucketfs_installer_preserves_public_policy(
    bucketfs_installer: Path,
    *,
    public: bool,
) -> None:
    (bucketfs_installer / "info.json").write_text(
        json.dumps(
            {
                "buckets": {
                    "default": {"public": public, "read_passwd": PRIVATE_RESPONSE}
                },
            }
        )
    )
    result = _run_installer(bucketfs_installer)
    assert result.returncode == 0
    assert json.loads((bucketfs_installer / "request.json").read_text()) == {
        "bucketfs_name": "bfsdefault",
        "bucket_name": "default",
        "public": public,
        "read_password": READ_PASSWORD,
        "write_password": WRITE_PASSWORD,
    }


@pytest.mark.parametrize(
    "failure", ["config", "connect", "bucketfs_info", "bucket_modify"]
)
def test_bucketfs_installer_sanitizes_command_failure(
    bucketfs_installer: Path,
    monkeypatch: pytest.MonkeyPatch,
    failure: str,
) -> None:
    monkeypatch.setenv("TEST_FAILURE", failure)
    result = _run_installer(bucketfs_installer)
    assert result.returncode != 0
    assert (
        "Failed to configure BucketFS credentials for bfsdefault/default"
        in result.stderr
    )
    if failure != "bucket_modify":
        assert not (bucketfs_installer / "request.json").exists()


@pytest.mark.parametrize("invalid", ["policy", "response", "password"])
def test_bucketfs_installer_rejects_invalid_values(
    bucketfs_installer: Path,
    invalid: str,
) -> None:
    if invalid == "policy":
        (bucketfs_installer / "info.json").write_text(
            '{"buckets":{"default":{"public":"false"}}}'
        )
    elif invalid == "response":
        (bucketfs_installer / "info.json").write_text("{broken")
    else:
        (bucketfs_installer / "infra.json").write_text(
            '{"bucketfsReadPasswordB64":null}'
        )
    result = _run_installer(bucketfs_installer)
    assert result.returncode != 0
    assert not (bucketfs_installer / "request.json").exists()
