# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT
# ruff: noqa: INP001

import argparse
import json
import re
import subprocess
import sys

SEMANTIC_VERSION = re.compile(
    r"^(0|[1-9][0-9]*)\."
    r"(0|[1-9][0-9]*)\."
    r"(0|[1-9][0-9]*)"
    r"(?:-((?:0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*)"
    r"(?:\.(?:0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*))*))?"
    r"(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$"
)
FULL_COMMIT = re.compile(r"[0-9a-fA-F]{40}")
VERSION_TAG = re.compile(r"(?:.*/)?v(?P<version>.+)")
MIKE_BRANCH = "gh-pages"
MKDOCS_CONFIG = "user-docs/mkdocs.yml"
PUBLISH_VERSION_ERROR = (
    "version is required when source_ref is not a tag ending in v<semver>"
)
DELETE_TARGET_ERROR = (
    "delete target must be a published semantic version such as 2.3.0-rc1"
)
DELETE_LATEST_ERROR = (
    "publish a newer stable version before deleting the version referenced by 'latest'"
)
VERSION_ERROR = "version must use semantic-version syntax such as 2.3.0 or 2.3.0-rc1"


class VersionError(Exception):
    pass


def run(*args: str, capture_output: bool = False) -> subprocess.CompletedProcess[str]:
    return subprocess.run(args, check=True, capture_output=capture_output, text=True)


def mike(
    command: str, *args: str, capture_output: bool = False
) -> subprocess.CompletedProcess[str]:
    return run(
        "mike",
        command,
        "--config-file",
        MKDOCS_CONFIG,
        *args,
        capture_output=capture_output,
    )


def validate_delete(target: str) -> str:
    if not SEMANTIC_VERSION.fullmatch(target):
        raise VersionError(DELETE_TARGET_ERROR)
    return target


def validate_publish(source_ref: str, version: str | None) -> str:
    if version:
        require_version(version)
        return version

    selected_tag = source_ref.removeprefix("refs/tags/")
    match = (
        None
        if FULL_COMMIT.fullmatch(source_ref)
        else VERSION_TAG.fullmatch(selected_tag)
    )
    derived = match.group("version") if match else ""
    if not SEMANTIC_VERSION.fullmatch(derived):
        raise VersionError(PUBLISH_VERSION_ERROR)
    return derived


def is_stable(version: str) -> bool:
    return "-" not in version.partition("+")[0]


def require_version(version: str) -> None:
    if not SEMANTIC_VERSION.fullmatch(version):
        raise VersionError(VERSION_ERROR)


def publish(version: str) -> None:
    require_version(version)
    arguments = [
        "--branch",
        MIKE_BRANCH,
        "--alias-type",
        "redirect",
    ]
    if is_stable(version):
        mike("deploy", *arguments, "--update-aliases", version, "latest")
        mike("set-default", "--branch", MIKE_BRANCH, "latest")
    else:
        mike("deploy", *arguments, version)


def delete(version: str) -> None:
    require_version(version)
    listed = mike(
        "list",
        "--branch",
        MIKE_BRANCH,
        "--json",
        capture_output=True,
    )
    metadata = next(
        (item for item in json.loads(listed.stdout) if item.get("version") == version),
        None,
    )
    if metadata is None:
        return
    if "latest" in metadata.get("aliases", []):
        raise VersionError(DELETE_LATEST_ERROR)

    mike("delete", "--branch", MIKE_BRANCH, version)
    mike("set-default", "--branch", MIKE_BRANCH, "latest")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Manage versioned user documentation locally"
    )
    commands = parser.add_subparsers(dest="command", required=True)

    validate_delete_parser = commands.add_parser(
        "validate-delete", help="Validate a deletion request"
    )
    validate_delete_parser.add_argument("target")

    validate_publish_parser = commands.add_parser(
        "validate-publish", help="Validate and normalize a publication request"
    )
    validate_publish_parser.add_argument("source_ref")
    validate_publish_parser.add_argument("--version")

    publish_parser = commands.add_parser(
        "publish", help="Publish a version to the local catalog"
    )
    publish_parser.add_argument("version")

    delete_parser = commands.add_parser(
        "delete", help="Delete a version from the local catalog"
    )
    delete_parser.add_argument("version")

    return parser.parse_args()


def main() -> int:
    args = parse_args()
    try:
        if args.command == "validate-delete":
            sys.stdout.write(f"{validate_delete(args.target)}\n")
        elif args.command == "validate-publish":
            sys.stdout.write(f"{validate_publish(args.source_ref, args.version)}\n")
        elif args.command == "publish":
            publish(args.version)
        else:
            delete(args.version)
    except VersionError as error:
        sys.stderr.write(f"error: {error}\n")
        return 1
    except subprocess.CalledProcessError as error:
        return error.returncode
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
