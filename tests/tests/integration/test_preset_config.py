# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

"""Preset flags, config round-trip, retry-safe re-init, and remove guards (offline)."""

import json
from pathlib import Path

import pytest

from tests.testcase_helpers import copy_named_no_resource_presets, run_command

DEFAULT_CLUSTER_SIZE = 1


CHANGED_CLUSTER_SIZE = 3


@pytest.mark.launcher_tests
def test_preset_flags_render_with_defaults(exasol_path: str) -> None:
    # When init help is rendered for a preset
    output = run_command([exasol_path, "init", "aws", "--help"]).stdout

    # Then preset-specific flags render with their defaults
    assert "--cluster-size" in output
    assert "--instance-type" in output
    assert "(default:" in output


@pytest.mark.launcher_tests
def test_config_set_get_reset_round_trip(exasol_path: str, tmp_path: Path) -> None:
    # Given an initialized deployment
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()
    base = ["--deployment-dir", str(deployment_dir), "--no-launcher-version-check"]
    run_command([exasol_path, "init", "aws", *base])

    dir_flag = ["--deployment-dir", str(deployment_dir)]

    # Then the default cluster-size is reported
    initial = json.loads(
        run_command(
            [exasol_path, "config", "get", "--json", "cluster-size", *dir_flag]
        ).stdout
    )
    assert initial["infrastructure"]["options"]["cluster-size"] == DEFAULT_CLUSTER_SIZE

    # When cluster-size is changed
    run_command([exasol_path, "config", "set", "--cluster-size", "3", *dir_flag])
    changed = json.loads(
        run_command(
            [exasol_path, "config", "get", "--json", "cluster-size", *dir_flag]
        ).stdout
    )
    assert changed["infrastructure"]["options"]["cluster-size"] == CHANGED_CLUSTER_SIZE

    # When the configuration is reset
    run_command([exasol_path, "config", "reset", "--all", *dir_flag])
    reset = json.loads(
        run_command(
            [exasol_path, "config", "get", "--json", "cluster-size", *dir_flag]
        ).stdout
    )
    assert reset["infrastructure"]["options"]["cluster-size"] == DEFAULT_CLUSTER_SIZE


EXPECTED_CLUSTER_SIZE = 2


@pytest.mark.launcher_tests
def test_reinit_same_preset_is_safe(exasol_path: str, tmp_path: Path) -> None:
    # Given a directory initialized with a preset
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()
    base = ["--deployment-dir", str(deployment_dir), "--no-launcher-version-check"]
    first = run_command([exasol_path, "init", "aws", *base])
    assert first.returncode == 0

    # When init is rerun with the same preset and a changed option
    second = run_command([exasol_path, "init", "aws", "--cluster-size", "2", *base])

    # Then it succeeds without error and applies the configuration change
    assert second.returncode == 0
    config = json.loads(
        run_command(
            [
                exasol_path,
                "config",
                "get",
                "--json",
                "cluster-size",
                "--deployment-dir",
                str(deployment_dir),
            ]
        ).stdout
    )
    assert config["infrastructure"]["options"]["cluster-size"] == EXPECTED_CLUSTER_SIZE


@pytest.mark.launcher_tests
def test_remove_auto_approve_deletes_dir(exasol_path: str, tmp_path: Path) -> None:
    # Given an initialized deployment directory
    infra_dir, install_dir = copy_named_no_resource_presets(
        tmp_path, "remove-only", "Remove Infrastructure", "Remove Installation"
    )
    deployment_dir = tmp_path / "deployment"
    run_command(
        [
            exasol_path,
            "init",
            str(infra_dir),
            str(install_dir),
            "--deployment-dir",
            str(deployment_dir),
            "--no-launcher-version-check",
        ]
    )

    # When remove runs with --auto-approve
    result = run_command(
        [
            exasol_path,
            "remove",
            "--auto-approve",
            "--deployment-dir",
            str(deployment_dir),
        ]
    )

    # Then the local directory is removed without destroying resources
    assert result.returncode == 0
    assert not deployment_dir.exists()
