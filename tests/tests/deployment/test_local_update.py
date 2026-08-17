# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

"""Historical macOS local deployment update tests."""

import hashlib
import shutil
import stat
import subprocess
import tarfile
from pathlib import Path, PurePosixPath
from typing import Final

import pytest
import requests

from tests.testcase_helpers import requires_macos_arm, run_command

HISTORICAL_LOCAL_VERSIONS: Final = ("2.1.0", "2.2.0")
RELEASE_DOWNLOAD_BASE_URL: Final = (
    "https://github.com/exasol/exasol-personal/releases/download"
)
MACOS_ARM64_ARCHIVE_NAME: Final = "exasol-personal_macOS_arm64.tar.gz"
DOWNLOAD_CHUNK_SIZE: Final = 1024 * 1024
DOWNLOAD_TIMEOUT_SECONDS: Final = 10 * 60
CHECKSUM_LINE_FIELDS: Final = 2

CREATE_TEST_DATA_SQL: Final = """
CREATE SCHEMA HISTORICAL_UPDATE;
CREATE TABLE HISTORICAL_UPDATE.PRESERVED_ROWS (
    ID DECIMAL(18, 0),
    LABEL VARCHAR(100)
);
INSERT INTO HISTORICAL_UPDATE.PRESERVED_ROWS VALUES
    (1, 'alpha'),
    (2, 'beta'),
    (3, 'gamma');
COMMIT;
"""
QUERY_TEST_DATA_SQL: Final = """
SELECT ID, LABEL
FROM HISTORICAL_UPDATE.PRESERVED_ROWS
ORDER BY ID
"""
EXPECTED_TEST_DATA_CSV: Final = "ID,LABEL\n1,alpha\n2,beta\n3,gamma\n"


def _download_text(url: str) -> str:
    response = requests.get(url, timeout=DOWNLOAD_TIMEOUT_SECONDS)
    response.raise_for_status()
    return response.text


def _expected_checksum(checksums: str, archive_name: str) -> str:
    matches = [
        checksum
        for line in checksums.splitlines()
        if len(parts := line.split()) == CHECKSUM_LINE_FIELDS
        and (checksum := parts[0])
        and parts[1] == archive_name
    ]
    if len(matches) != 1:
        msg = f"expected exactly one checksum for {archive_name}, found {len(matches)}"
        raise ValueError(msg)
    return matches[0]


def _download_archive(url: str, destination: Path, expected_checksum: str) -> None:
    digest = hashlib.sha256()
    with requests.get(
        url,
        stream=True,
        timeout=DOWNLOAD_TIMEOUT_SECONDS,
    ) as response:
        response.raise_for_status()
        with destination.open("wb") as archive:
            for chunk in response.iter_content(chunk_size=DOWNLOAD_CHUNK_SIZE):
                if chunk:
                    digest.update(chunk)
                    archive.write(chunk)

    actual_checksum = digest.hexdigest()
    if actual_checksum != expected_checksum:
        msg = (
            f"checksum mismatch for {destination.name}: "
            f"expected {expected_checksum}, got {actual_checksum}"
        )
        raise ValueError(msg)


def _extract_launcher(archive_path: Path, launcher_path: Path) -> None:
    with tarfile.open(archive_path, mode="r:gz") as archive:
        launcher_members = [
            member
            for member in archive.getmembers()
            if member.isfile() and PurePosixPath(member.name).name == "exasol"
        ]
        if len(launcher_members) != 1:
            msg = (
                "expected exactly one exasol launcher in "
                f"{archive_path.name}, found {len(launcher_members)}"
            )
            raise ValueError(msg)

        launcher_file = archive.extractfile(launcher_members[0])
        if launcher_file is None:
            msg = f"could not read launcher from {archive_path.name}"
            raise ValueError(msg)
        with launcher_file, launcher_path.open("wb") as destination:
            shutil.copyfileobj(launcher_file, destination)

    launcher_path.chmod(
        launcher_path.stat().st_mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH
    )


def _historical_launcher(version: str, download_dir: Path) -> Path:
    release_url = f"{RELEASE_DOWNLOAD_BASE_URL}/v{version}"
    checksums_name = f"exasol-personal_{version}_checksums.txt"
    checksums = _download_text(f"{release_url}/{checksums_name}")
    expected_checksum = _expected_checksum(checksums, MACOS_ARM64_ARCHIVE_NAME)

    archive_path = download_dir / MACOS_ARM64_ARCHIVE_NAME
    _download_archive(
        f"{release_url}/{MACOS_ARM64_ARCHIVE_NAME}",
        archive_path,
        expected_checksum,
    )

    launcher_path = download_dir / f"exasol-{version}"
    _extract_launcher(archive_path, launcher_path)
    assert run_command([str(launcher_path), "version"]).stdout.strip() == version
    return launcher_path


def _cleanup_deployment(
    deployment_dir: Path,
    current_launcher: Path,
    historical_launcher: Path,
    *,
    handoff_completed: bool,
) -> None:
    base_args = ["--deployment-dir", str(deployment_dir)]
    cleanup_args = ["destroy", "--remove", "--auto-approve", *base_args]
    try:
        run_command([str(current_launcher), *cleanup_args])
    except subprocess.CalledProcessError as current_cleanup_error:
        if handoff_completed:
            raise
        try:
            run_command([str(historical_launcher), *cleanup_args])
        except subprocess.CalledProcessError:
            raise current_cleanup_error from None


@pytest.mark.parametrize("historical_version", HISTORICAL_LOCAL_VERSIONS)
@pytest.mark.local_e2e
@pytest.mark.installation_e2e
@requires_macos_arm
def test_historical_local_update_preserves_committed_data(
    historical_version: str,
    exasol_path: str,
    tmp_path: Path,
) -> None:
    # Given a fresh local deployment installed by a verified historical launcher
    current_launcher = Path(exasol_path).resolve()
    historical_launcher = _historical_launcher(historical_version, tmp_path)
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()
    target_args = ["--deployment-dir", str(deployment_dir)]
    handoff_completed = False
    original_error: BaseException | None = None

    try:
        run_command(
            [
                str(historical_launcher),
                "install",
                "local",
                "--no-launcher-version-check",
                *target_args,
            ]
        )
        run_command(
            [
                str(historical_launcher),
                "connect",
                "-c",
                CREATE_TEST_DATA_SQL,
                *target_args,
            ]
        )
        run_command([str(historical_launcher), "stop", *target_args])

        # When the current launcher starts the stopped historical deployment
        run_command([str(current_launcher), "start", *target_args])
        handoff_completed = True
        result = run_command(
            [
                str(current_launcher),
                "connect",
                "--csv",
                "-c",
                QUERY_TEST_DATA_SQL,
                *target_args,
            ]
        )

        # Then the explicitly committed rows survive runner replacement and migration
        assert result.stdout == EXPECTED_TEST_DATA_CSV
    except BaseException as error:
        original_error = error
        raise
    finally:
        try:
            _cleanup_deployment(
                deployment_dir,
                current_launcher,
                historical_launcher,
                handoff_completed=handoff_completed,
            )
        except subprocess.CalledProcessError:
            if original_error is None:
                raise
