# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

"""Launcher tests for external preset sources (file://, archives, git, errors)."""

import functools
import http.server
import json
import os
import shutil
import subprocess
import tarfile
import threading
import urllib.parse
import zipfile
from collections.abc import Iterator
from pathlib import Path
from subprocess import CalledProcessError, CompletedProcess
from typing import Final

import pytest

from .helpers import (
    export_preset,
    first_infrastructure_preset_id_or_skip,
    installation_preset_id_or_skip,
    preset_id_or_skip,
    run_command,
)

SUBPATH_PRESET_IDS: Final = ("aws", "azure")
GIT_COMMIT_IDENTITY: Final = (
    "-c",
    "user.name=Exasol Test",
    "-c",
    "user.email=test@example.com",
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


def test_file_uri_plain_file_without_manifest_returns_error(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given a plain file that isn't a preset directory or archive, and so
    # can't contain infrastructure.yaml
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

    # Then it fails with an error about the missing infrastructure manifest
    assert exc.value.returncode != 0
    assert "infrastructure manifest" in exc.value.stderr


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


def _manifest_name(preset_dir: Path) -> str:
    for line in (preset_dir / "infrastructure.yaml").read_text().splitlines():
        key, _, value = line.partition(":")
        if key == "name":
            return value.strip().strip('"')
    msg = f"no name in {preset_dir / 'infrastructure.yaml'}"
    raise AssertionError(msg)


def _export_multi_preset_tree(exasol_path: str, root: Path) -> dict[str, str]:
    """Export two infrastructure presets under infra/<id>; map subpath to name."""
    names: dict[str, str] = {}
    for preset_id in SUBPATH_PRESET_IDS:
        preset_id_or_skip(exasol_path, "infrastructures", preset_id)
        preset_dir = root / "infra" / preset_id
        preset_dir.mkdir(parents=True)
        export_preset(exasol_path, preset_id, "infrastructure", str(preset_dir))
        names[f"infra/{preset_id}"] = _manifest_name(preset_dir)

    return names


def _archive_tree(root: Path, archive_path: Path) -> None:
    files = [path for path in root.rglob("*") if path.is_file()]
    if archive_path.suffix == ".zip":
        with zipfile.ZipFile(archive_path, "w", zipfile.ZIP_DEFLATED) as zf:
            for file_path in files:
                zf.write(str(file_path), str(file_path.relative_to(root)))
        return
    with tarfile.open(archive_path, "w:gz") as tar:
        for file_path in files:
            tar.add(str(file_path), arcname=str(file_path.relative_to(root)))


def _init_from(
    exasol_path: str, source: str, deployment_dir: Path
) -> CompletedProcess[str]:
    return subprocess.run(
        [
            exasol_path,
            "init",
            source,
            "--deployment-dir",
            str(deployment_dir),
            "--no-launcher-version-check",
        ],
        capture_output=True,
        text=True,
        encoding="utf-8",
        check=False,
    )


def _selected_infrastructure_name(exasol_path: str, deployment_dir: Path) -> str:
    result = run_command(
        [
            exasol_path,
            "config",
            "get",
            "--json",
            "--deployment-dir",
            str(deployment_dir),
        ]
    )
    return str(json.loads(result.stdout)["infrastructure"]["identity"]["displayName"])


def _assert_selects(
    exasol_path: str, source: str, deployment_dir: Path, expected_name: str
) -> None:
    result = _init_from(exasol_path, source, deployment_dir)
    assert result.returncode == 0, result.stderr
    assert _selected_infrastructure_name(exasol_path, deployment_dir) == expected_name


@pytest.mark.openspec("preset-source-fetching")
def test_subpath_fragment_selects_preset_from_directory_uri(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given a directory holding two presets in separate subdirectories
    tree = tmp_path / "presets"
    names = _export_multi_preset_tree(exasol_path, tree)

    for subpath, expected_name in names.items():
        # When init selects one of them through a fragment
        # Then that subdirectory's preset is used rather than the directory root
        _assert_selects(
            exasol_path,
            f"file://{tree}#{subpath}",
            tmp_path / f"deployment-{subpath.replace('/', '-')}",
            expected_name,
        )


@pytest.mark.openspec("preset-source-fetching")
@pytest.mark.parametrize(
    "archive_name", ["presets.tar.gz", "presets.tgz", "presets.zip"]
)
def test_subpath_fragment_selects_preset_from_archive_uri(
    exasol_path: str, tmp_path: Path, archive_name: str
) -> None:
    # Given an archive holding two presets in separate subdirectories
    tree = tmp_path / "presets"
    names = _export_multi_preset_tree(exasol_path, tree)
    archive_path = tmp_path / archive_name
    _archive_tree(tree, archive_path)

    for subpath, expected_name in names.items():
        # When init selects one of them through a fragment
        # Then the named subdirectory of the extracted archive is used
        _assert_selects(
            exasol_path,
            f"file://{archive_path}#{subpath}",
            tmp_path / f"deployment-{subpath.replace('/', '-')}",
            expected_name,
        )


@pytest.mark.openspec("preset-source-fetching")
def test_subpath_fragment_selects_preset_from_http_archive(
    exasol_path: str,
    tmp_path: Path,
    http_file_server: tuple[Path, str],
) -> None:
    # Given a multi-preset archive served over HTTP
    serve_dir, base_url = http_file_server
    tree = tmp_path / "presets"
    names = _export_multi_preset_tree(exasol_path, tree)
    _archive_tree(tree, serve_dir / "presets.tar.gz")
    subpath = "infra/azure"

    # When init selects a preset through a fragment on the remote URL
    # Then the named subdirectory is used
    _assert_selects(
        exasol_path,
        f"{base_url}/presets.tar.gz#{subpath}",
        tmp_path / "deployment",
        names[subpath],
    )


@pytest.mark.openspec("preset-source-fetching")
def test_percent_encoded_subpath_fragment_is_decoded(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given a multi-preset archive
    tree = tmp_path / "presets"
    names = _export_multi_preset_tree(exasol_path, tree)
    archive_path = tmp_path / "presets.tar.gz"
    _archive_tree(tree, archive_path)

    # When the fragment encodes its path separator
    # Then it resolves to the same preset as the unencoded fragment
    _assert_selects(
        exasol_path,
        f"file://{archive_path}#infra%2Fazure",
        tmp_path / "deployment",
        names["infra/azure"],
    )


@pytest.mark.openspec("preset-source-fetching")
def test_missing_subpath_fragment_reports_missing_manifest(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given a multi-preset archive without the requested subdirectory
    tree = tmp_path / "presets"
    _export_multi_preset_tree(exasol_path, tree)
    archive_path = tmp_path / "presets.tar.gz"
    _archive_tree(tree, archive_path)
    deployment_dir = tmp_path / "deployment"

    # When init selects a subdirectory that does not exist
    result = _init_from(
        exasol_path, f"file://{archive_path}#infra/missing", deployment_dir
    )

    # Then it fails naming the subpath and the missing manifest, without initializing
    assert result.returncode != 0
    assert "infra/missing" in result.stderr
    assert "infrastructure manifest" in result.stderr
    assert not (deployment_dir / ".exasolLauncherState.json").exists()


class _GitSmartHTTPHandler(http.server.BaseHTTPRequestHandler):
    """Serve bare repositories through `git http-backend` (smart HTTP)."""

    project_root: str = ""

    def _serve(self) -> None:
        url = urllib.parse.urlsplit(self.path)
        body = self.rfile.read(int(self.headers.get("Content-Length") or 0))
        env = {
            **os.environ,
            "GIT_PROJECT_ROOT": self.project_root,
            "GIT_HTTP_EXPORT_ALL": "1",
            "PATH_INFO": url.path,
            "QUERY_STRING": url.query,
            "REQUEST_METHOD": self.command,
            "CONTENT_TYPE": self.headers.get("Content-Type", ""),
            "CONTENT_LENGTH": str(len(body)),
        }
        if protocol := self.headers.get("Git-Protocol"):
            env["GIT_PROTOCOL"] = protocol
        output = subprocess.run(
            ["git", "http-backend"],  # noqa: S607
            input=body,
            env=env,
            capture_output=True,
            check=True,
        ).stdout
        raw_headers, _, payload = output.partition(b"\r\n\r\n")
        status = 200
        headers: list[tuple[str, str]] = []
        for line in raw_headers.decode().split("\r\n"):
            name, _, value = line.partition(":")
            if name.lower() == "status":
                status = int(value.split()[0])
            else:
                headers.append((name, value.strip()))
        self.send_response(status)
        for name, value in headers:
            self.send_header(name, value)
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)

    do_GET = _serve  # noqa: N815
    do_POST = _serve  # noqa: N815

    def log_message(self, format: str, *args: object) -> None:  # noqa: A002
        pass


def _git(*args: str, cwd: Path) -> None:
    subprocess.run(
        ["git", *GIT_COMMIT_IDENTITY, *args],  # noqa: S607
        cwd=cwd,
        capture_output=True,
        text=True,
        check=True,
    )


@pytest.fixture
def git_http_server(tmp_path: Path) -> Iterator[tuple[Path, str]]:
    """Serve bare repositories placed in the returned directory over smart HTTP."""
    if shutil.which("git") is None:
        pytest.skip("git is not installed")
    exec_path = subprocess.run(
        ["git", "--exec-path"],  # noqa: S607
        capture_output=True,
        text=True,
        check=True,
    ).stdout.strip()
    if not any(Path(exec_path).glob("git-http-backend*")):
        pytest.skip("git http-backend is not available")

    repos_dir = tmp_path / "git_served"
    repos_dir.mkdir()
    handler = type("Handler", (_GitSmartHTTPHandler,), {"project_root": str(repos_dir)})
    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), handler)
    port = server.server_address[1]
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        yield repos_dir, f"http://127.0.0.1:{port}"
    finally:
        server.shutdown()


def _publish_bare_repo(work_dir: Path, repos_dir: Path, name: str) -> None:
    subprocess.run(
        ["git", "clone", "--quiet", "--bare", str(work_dir), str(repos_dir / name)],  # noqa: S607
        capture_output=True,
        text=True,
        check=True,
    )


@pytest.mark.openspec("preset-source-fetching")
def test_init_accepts_git_http_preset_with_and_without_ref(
    exasol_path: str,
    tmp_path: Path,
    git_http_server: tuple[Path, str],
) -> None:
    # Given a git repository whose root is an infrastructure preset, tagged v1
    repos_dir, base_url = git_http_server
    infra_id = first_infrastructure_preset_id_or_skip(exasol_path)
    work_dir = tmp_path / "work"
    work_dir.mkdir()
    export_preset(exasol_path, infra_id, "infrastructure", str(work_dir))
    expected_name = _manifest_name(work_dir)
    _git("init", "--quiet", "-b", "main", cwd=work_dir)
    _git("add", "-A", cwd=work_dir)
    _git("commit", "--quiet", "-m", "preset", cwd=work_dir)
    _git("tag", "v1", cwd=work_dir)
    _publish_bare_repo(work_dir, repos_dir, "preset.git")

    # When init resolves the repository at its default branch and at the tag
    # Then both resolve to the preset
    _assert_selects(
        exasol_path,
        f"{base_url}/preset.git",
        tmp_path / "deployment-default",
        expected_name,
    )
    _assert_selects(
        exasol_path,
        f"{base_url}/preset.git@v1",
        tmp_path / "deployment-tag",
        expected_name,
    )


@pytest.mark.openspec("preset-source-fetching")
def test_git_ref_and_subpath_select_preset_content_at_that_ref(
    exasol_path: str,
    tmp_path: Path,
    git_http_server: tuple[Path, str],
) -> None:
    # Given a repository with two presets at tag v1, one removed afterwards
    repos_dir, base_url = git_http_server
    work_dir = tmp_path / "work"
    names = _export_multi_preset_tree(exasol_path, work_dir)
    _git("init", "--quiet", "-b", "main", cwd=work_dir)
    _git("add", "-A", cwd=work_dir)
    _git("commit", "--quiet", "-m", "two presets", cwd=work_dir)
    _git("tag", "v1", cwd=work_dir)
    _git("rm", "-r", "--quiet", "infra/aws", cwd=work_dir)
    _git("commit", "--quiet", "-m", "remove aws", cwd=work_dir)
    _publish_bare_repo(work_dir, repos_dir, "presets.git")
    repo_url = f"{base_url}/presets.git"

    # When the tag and a subpath are selected together
    # Then the subdirectory is taken from the tagged commit
    _assert_selects(
        exasol_path,
        f"{repo_url}@v1#infra/aws",
        tmp_path / "deployment-v1-aws",
        names["infra/aws"],
    )

    # When only a subpath is selected
    # Then it resolves against the default branch
    _assert_selects(
        exasol_path,
        f"{repo_url}#infra/azure",
        tmp_path / "deployment-main-azure",
        names["infra/azure"],
    )

    # When the default branch no longer contains the selected subpath
    result = _init_from(exasol_path, f"{repo_url}#infra/aws", tmp_path / "gone")

    # Then resolution fails with the missing manifest
    assert result.returncode != 0
    assert "infrastructure manifest" in result.stderr
