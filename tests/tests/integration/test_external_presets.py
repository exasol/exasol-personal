# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

"""Integration tests for external preset sources (file://, archives, error cases)."""

import functools
import http.server
import tarfile
import threading
import zipfile
from collections.abc import Iterator
from pathlib import Path
from subprocess import CalledProcessError

import pytest

from .helpers import (
    export_preset,
    first_infrastructure_preset_id_or_skip,
    installation_preset_id_or_skip,
    run_command,
)


@pytest.fixture
def http_file_server(tmp_path: Path) -> Iterator[tuple[Path, str]]:
    """Serve files from a subdirectory over HTTP on a random local port."""
    serve_dir = tmp_path / "http_served"
    serve_dir.mkdir()
    handler = functools.partial(
        http.server.SimpleHTTPRequestHandler,
        directory=str(serve_dir),
    )
    server = http.server.HTTPServer(("127.0.0.1", 0), handler)
    port = server.server_address[1]
    thread = threading.Thread(target=server.serve_forever)
    thread.daemon = True
    thread.start()
    try:
        yield serve_dir, f"http://127.0.0.1:{port}"
    finally:
        server.shutdown()


def test_init_accepts_file_uri_directory_infra_preset(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given an infrastructure preset exported to a local directory
    infra_id = first_infrastructure_preset_id_or_skip(exasol_path)
    preset_dir = tmp_path / "preset"
    preset_dir.mkdir()
    export_preset(exasol_path, infra_id, "infrastructure", str(preset_dir))

    # Given an empty deployment directory
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()

    # When init is invoked with a file:// URI pointing to that directory
    result = run_command(
        [
            exasol_path,
            "init",
            f"file://{preset_dir}",
            "--deployment-dir",
            str(deployment_dir),
        ]
    )

    # Then it succeeds and creates the deployment state file
    assert result.returncode == 0
    assert (deployment_dir / ".exasolLauncherState.json").exists()


def test_init_accepts_file_uri_tar_gz_infra_preset(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given an infrastructure preset exported to a local directory
    infra_id = first_infrastructure_preset_id_or_skip(exasol_path)
    preset_dir = tmp_path / "preset"
    preset_dir.mkdir()
    export_preset(exasol_path, infra_id, "infrastructure", str(preset_dir))

    # Given the preset directory archived as a .tar.gz
    archive_path = tmp_path / "preset.tar.gz"
    with tarfile.open(archive_path, "w:gz") as tar:
        for file_path in preset_dir.rglob("*"):
            if file_path.is_file():
                tar.add(str(file_path), arcname=str(file_path.relative_to(preset_dir)))

    # Given an empty deployment directory
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()

    # When init is invoked with a file:// URI pointing to the archive
    result = run_command(
        [
            exasol_path,
            "init",
            f"file://{archive_path}",
            "--deployment-dir",
            str(deployment_dir),
        ]
    )

    # Then it succeeds and creates the deployment state file
    assert result.returncode == 0
    assert (deployment_dir / ".exasolLauncherState.json").exists()


def test_init_accepts_file_uri_zip_infra_preset(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given an infrastructure preset exported to a local directory
    infra_id = first_infrastructure_preset_id_or_skip(exasol_path)
    preset_dir = tmp_path / "preset"
    preset_dir.mkdir()
    export_preset(exasol_path, infra_id, "infrastructure", str(preset_dir))

    # Given the preset directory archived as a .zip
    archive_path = tmp_path / "preset.zip"
    with zipfile.ZipFile(archive_path, "w", zipfile.ZIP_DEFLATED) as zf:
        for file_path in preset_dir.rglob("*"):
            if file_path.is_file():
                zf.write(str(file_path), str(file_path.relative_to(preset_dir)))

    # Given an empty deployment directory
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()

    # When init is invoked with a file:// URI pointing to the archive
    result = run_command(
        [
            exasol_path,
            "init",
            f"file://{archive_path}",
            "--deployment-dir",
            str(deployment_dir),
        ]
    )

    # Then it succeeds and creates the deployment state file
    assert result.returncode == 0
    assert (deployment_dir / ".exasolLauncherState.json").exists()


def test_init_accepts_file_uri_directory_install_preset(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given both preset types exported to local directories
    # ubuntu is used for the install preset because it is compatible with the
    # first available infrastructure preset (same pairing as the existing
    # test_init_accepts_install_preset_path_as_second_arg test).
    infra_id = first_infrastructure_preset_id_or_skip(exasol_path)
    install_id = installation_preset_id_or_skip(exasol_path, "ubuntu")

    infra_dir = tmp_path / "infra"
    infra_dir.mkdir()
    export_preset(exasol_path, infra_id, "infrastructure", str(infra_dir))

    install_dir = tmp_path / "install"
    install_dir.mkdir()
    export_preset(exasol_path, install_id, "installation", str(install_dir))

    # Given an empty deployment directory
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()

    # When init is invoked with file:// URIs for both preset positions
    result = run_command(
        [
            exasol_path,
            "init",
            f"file://{infra_dir}",
            f"file://{install_dir}",
            "--deployment-dir",
            str(deployment_dir),
        ]
    )

    # Then it succeeds
    assert result.returncode == 0
    assert (deployment_dir / ".exasolLauncherState.json").exists()


def test_unknown_preset_name_error_includes_available_names(
    exasol_path: str,
) -> None:
    # When init is invoked with an unknown preset name
    with pytest.raises(CalledProcessError) as exc:
        run_command([exasol_path, "init", "this-preset-does-not-exist"])

    # Then it fails and names the unknown preset in the error
    assert exc.value.returncode != 0
    assert "this-preset-does-not-exist" in exc.value.stderr

    # And it lists the available embedded preset names
    assert "available" in exc.value.stderr.lower()


def test_file_uri_nonexistent_path_returns_error(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given a deployment directory
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()

    # When init is invoked with a file:// URI pointing to a path that does not exist
    args = [
        exasol_path,
        "init",
        "file:///this/path/does/not/exist",
        "--deployment-dir",
        str(deployment_dir),
    ]
    with pytest.raises(CalledProcessError) as exc:
        run_command(args)

    # Then it fails with an error that references the missing path
    assert exc.value.returncode != 0
    assert "does not exist" in exc.value.stderr or "not exist" in exc.value.stderr


def test_file_uri_unsupported_file_type_returns_error(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given a plain file (not a directory or a supported archive)
    plain_file = tmp_path / "preset.yaml"
    plain_file.write_text("kind: infrastructure\n")

    # Given a deployment directory
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()

    # When init is invoked with a file:// URI pointing to that plain file
    args = [
        exasol_path,
        "init",
        f"file://{plain_file}",
        "--deployment-dir",
        str(deployment_dir),
    ]
    with pytest.raises(CalledProcessError) as exc:
        run_command(args)

    # Then it fails with an error mentioning the directory/archive requirement
    assert exc.value.returncode != 0
    assert "directory" in exc.value.stderr or "archive" in exc.value.stderr


def test_at_ref_on_non_git_url_returns_error(exasol_path: str, tmp_path: Path) -> None:
    # Given a deployment directory
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()

    # When init is invoked with an @ref suffix on a non-git HTTPS URL
    args = [
        exasol_path,
        "init",
        "https://example.com/preset.tar.gz@v1.0.0",
        "--deployment-dir",
        str(deployment_dir),
    ]
    with pytest.raises(CalledProcessError) as exc:
        run_command(args)

    # Then it fails with an error about the @ref syntax restriction
    assert exc.value.returncode != 0
    assert "@ref" in exc.value.stderr


def test_init_accepts_http_tar_gz_infra_preset(
    exasol_path: str,
    tmp_path: Path,
    http_file_server: tuple[Path, str],
) -> None:
    # Given an infrastructure preset archived as .tar.gz and served over HTTP
    infra_id = first_infrastructure_preset_id_or_skip(exasol_path)
    serve_dir, base_url = http_file_server

    preset_dir = serve_dir / "preset"
    preset_dir.mkdir()
    export_preset(exasol_path, infra_id, "infrastructure", str(preset_dir))

    archive_path = serve_dir / "preset.tar.gz"
    with tarfile.open(archive_path, "w:gz") as tar:
        for file_path in preset_dir.rglob("*"):
            if file_path.is_file():
                tar.add(str(file_path), arcname=str(file_path.relative_to(preset_dir)))

    # Given an empty deployment directory
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()

    # When init is invoked with an http:// URL pointing to the archive
    result = run_command(
        [
            exasol_path,
            "init",
            f"{base_url}/preset.tar.gz",
            "--deployment-dir",
            str(deployment_dir),
        ]
    )

    # Then it succeeds and creates the deployment state file
    assert result.returncode == 0
    assert (deployment_dir / ".exasolLauncherState.json").exists()


def test_init_accepts_http_zip_infra_preset(
    exasol_path: str,
    tmp_path: Path,
    http_file_server: tuple[Path, str],
) -> None:
    # Given an infrastructure preset archived as .zip and served over HTTP
    infra_id = first_infrastructure_preset_id_or_skip(exasol_path)
    serve_dir, base_url = http_file_server

    preset_dir = serve_dir / "preset"
    preset_dir.mkdir()
    export_preset(exasol_path, infra_id, "infrastructure", str(preset_dir))

    archive_path = serve_dir / "preset.zip"
    with zipfile.ZipFile(archive_path, "w", zipfile.ZIP_DEFLATED) as zf:
        for file_path in preset_dir.rglob("*"):
            if file_path.is_file():
                zf.write(str(file_path), str(file_path.relative_to(preset_dir)))

    # Given an empty deployment directory
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()

    # When init is invoked with an http:// URL pointing to the archive
    result = run_command(
        [
            exasol_path,
            "init",
            f"{base_url}/preset.zip",
            "--deployment-dir",
            str(deployment_dir),
        ]
    )

    # Then it succeeds and creates the deployment state file
    assert result.returncode == 0
    assert (deployment_dir / ".exasolLauncherState.json").exists()
