# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

import subprocess
from collections.abc import Callable

import pytest

from scripts import versions


@pytest.mark.parametrize(
    "target",
    [
        "1.2.3",
        "1.2.3-alpha--1",
        "1.2.3--",
        "1.2.3+001.build",
    ],
)
def test_validate_delete_accepts_semantic_versions(target: str) -> None:
    # Given / When
    actual = versions.validate_delete(target)

    # Then
    assert actual == target


@pytest.mark.parametrize(
    "target",
    [
        "v1.2.3",
        "01.2.3",
        "1.02.3",
        "1.2.03",
        "1.2.3-01",
        "1.2.3-alpha..1",
        "1.2.3-alpha_1",
    ],
)
def test_validate_delete_rejects_invalid_versions(target: str) -> None:
    # Given / When / Then
    with pytest.raises(versions.VersionError, match="delete target"):
        versions.validate_delete(target)


@pytest.mark.parametrize(
    ("source_ref", "expected"),
    [
        ("v2.3.0", "2.3.0"),
        ("docs/v2.2.0-rc.1", "2.2.0-rc.1"),
        ("refs/tags/docs/v2.1.0", "2.1.0"),
    ],
)
def test_validate_publish_derives_version_from_conventional_tags(
    source_ref: str, expected: str
) -> None:
    # Given / When
    actual = versions.validate_publish(source_ref, None)

    # Then
    assert actual == expected


@pytest.mark.parametrize("source_ref", ["a" * 40, "release-2.2.0"])
def test_validate_publish_requires_version_when_it_cannot_be_derived(
    source_ref: str,
) -> None:
    # Given / When / Then
    with pytest.raises(versions.VersionError, match="version is required"):
        versions.validate_publish(source_ref, None)


def test_validate_publish_prefers_explicit_version() -> None:
    # Given / When
    actual = versions.validate_publish("v9.9.9", "2.2.0")

    # Then
    assert actual == "2.2.0"


def test_validate_publish_rejects_invalid_explicit_version() -> None:
    # Given / When / Then
    with pytest.raises(versions.VersionError, match="semantic-version syntax"):
        versions.validate_publish("v2.2.0", "release-2.2")


def test_publish_stable_version_updates_latest(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    # Given
    calls: list[tuple[str, ...]] = []
    monkeypatch.setattr(versions, "mike", lambda *args, **_kwargs: calls.append(args))

    # When
    versions.publish("2.3.0")

    # Then
    assert calls == [
        (
            "deploy",
            "--branch",
            "gh-pages",
            "--alias-type",
            "redirect",
            "--update-aliases",
            "2.3.0",
            "latest",
        ),
        ("set-default", "--branch", "gh-pages", "latest"),
    ]


def test_publish_prerelease_preserves_latest(monkeypatch: pytest.MonkeyPatch) -> None:
    # Given
    calls: list[tuple[str, ...]] = []
    monkeypatch.setattr(versions, "mike", lambda *args, **_kwargs: calls.append(args))

    # When
    versions.publish("2.3.0-rc.1")

    # Then
    assert calls == [
        (
            "deploy",
            "--branch",
            "gh-pages",
            "--alias-type",
            "redirect",
            "2.3.0-rc.1",
        )
    ]


def mike_with_versions(
    catalog: list[dict[str, object]], calls: list[tuple[str, ...]]
) -> Callable[..., subprocess.CompletedProcess[str]]:
    def fake_mike(
        *args: str, capture_output: bool = False
    ) -> subprocess.CompletedProcess[str]:
        calls.append(args)
        stdout = versions.json.dumps(catalog) if capture_output else ""
        return subprocess.CompletedProcess(args, 0, stdout=stdout)

    return fake_mike


def test_delete_rejects_version_referenced_by_latest(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    # Given
    calls: list[tuple[str, ...]] = []
    catalog = [{"version": "2.3.0", "aliases": ["latest"]}]
    monkeypatch.setattr(versions, "mike", mike_with_versions(catalog, calls))

    # When / Then
    with pytest.raises(versions.VersionError, match="newer stable version"):
        versions.delete("2.3.0")
    assert calls == [("list", "--branch", "gh-pages", "--json")]


def test_delete_removes_only_selected_version(monkeypatch: pytest.MonkeyPatch) -> None:
    # Given
    calls: list[tuple[str, ...]] = []
    catalog = [
        {"version": "2.3.0", "aliases": ["latest"]},
        {"version": "2.4.0-rc.1", "aliases": []},
    ]
    monkeypatch.setattr(versions, "mike", mike_with_versions(catalog, calls))

    # When
    versions.delete("2.4.0-rc.1")

    # Then
    assert calls == [
        ("list", "--branch", "gh-pages", "--json"),
        ("delete", "--branch", "gh-pages", "2.4.0-rc.1"),
        ("set-default", "--branch", "gh-pages", "latest"),
    ]


def test_delete_retry_accepts_an_already_absent_version(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    # Given
    calls: list[tuple[str, ...]] = []
    catalog = [{"version": "2.3.0", "aliases": ["latest"]}]
    monkeypatch.setattr(versions, "mike", mike_with_versions(catalog, calls))

    # When
    versions.delete("2.4.0-rc.1")

    # Then
    assert calls == [("list", "--branch", "gh-pages", "--json")]


def test_publish_retry_reapplies_the_same_catalog_update(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    # Given
    calls: list[tuple[str, ...]] = []
    monkeypatch.setattr(versions, "mike", lambda *args, **_kwargs: calls.append(args))
    expected = (
        "deploy",
        "--branch",
        "gh-pages",
        "--alias-type",
        "redirect",
        "2.3.0-rc.1",
    )

    # When
    versions.publish("2.3.0-rc.1")
    versions.publish("2.3.0-rc.1")

    # Then
    assert calls == [expected, expected]
