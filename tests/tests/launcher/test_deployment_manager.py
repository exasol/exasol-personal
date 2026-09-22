# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

from pathlib import Path
from subprocess import CalledProcessError, CompletedProcess, TimeoutExpired
from types import SimpleNamespace
from typing import cast

import pytest

from framework.deployment import (
    Deployment,
    DeploymentManager,
    StatusDatabaseReady,
    StatusInitialized,
)
from framework.launcher import DeploymentConfig, Launcher
from tests.conftest import (
    _evidence_summary,
    _execution_boundary,
    _marker_values,
    _requires_cloud_opt_in,
    _validate_launcher_boundary,
    _xdist_group,
)


class FakeLauncher:
    def __init__(self) -> None:
        self.statuses: dict[str, str] = {}
        self.configs: dict[str, DeploymentConfig] = {}
        self.init_args: dict[str, tuple[str, ...]] = {}
        self.deploy_error: Exception | None = None
        self.ready_after_deploy = True
        self.destroy_failures = 0
        self.destroy_calls: list[str] = []
        self.start_calls: list[str] = []

    def init(
        self,
        deployment_dir: str,
        *args: str,
        config: DeploymentConfig,
    ) -> CompletedProcess[str]:
        self.statuses[deployment_dir] = StatusInitialized
        self.configs[deployment_dir] = config
        self.init_args[deployment_dir] = args
        return CompletedProcess(args, 0, "", "")

    def deploy(
        self, deployment_dir: str, *args: str, timeout: float | None = None
    ) -> CompletedProcess[str]:
        assert timeout == DeploymentManager.DEPLOY_TIMEOUT_SECONDS
        if self.deploy_error is not None:
            raise self.deploy_error
        self.statuses[deployment_dir] = (
            StatusDatabaseReady if self.ready_after_deploy else "not-ready"
        )
        return CompletedProcess(args, 0, "", "")

    def destroy(
        self, deployment_dir: str, *args: str, timeout: float | None = None
    ) -> CompletedProcess[str]:
        assert timeout == Deployment.DESTROY_TIMEOUT_SECONDS
        self.destroy_calls.append(deployment_dir)
        if self.destroy_failures:
            self.destroy_failures -= 1
            raise CalledProcessError(1, ("destroy", *args))
        self.statuses[deployment_dir] = StatusInitialized
        return CompletedProcess(args, 0, "", "")

    def start(self, deployment_dir: str, *args: str) -> CompletedProcess[str]:
        self.start_calls.append(deployment_dir)
        self.statuses[deployment_dir] = StatusDatabaseReady
        return CompletedProcess(args, 0, "", "")

    def has_status(self, deployment_dir: str, expected_status: str) -> bool:
        return self.statuses[deployment_dir] == expected_status

    def has_no_deployment(self, deployment_dir: str) -> bool:
        return self.statuses[deployment_dir] == StatusInitialized


class MarkedItem:
    def __init__(
        self,
        marker_name: str,
        *values: str,
        path: str = "tests/launcher/test_example.py",
    ) -> None:
        self.marker_name = marker_name
        self.values = values
        self.path = Path(path)

    def get_closest_marker(self, name: str) -> SimpleNamespace | None:
        if name != self.marker_name:
            return None
        return SimpleNamespace(args=self.values)


def test_manager_owns_each_deployment_boundary() -> None:
    # Given
    launcher = FakeLauncher()
    manager = DeploymentManager(
        cast("Launcher", launcher),
        DeploymentConfig(infra="aws", cluster_size=2),
    )

    # When
    controlled = manager.controlled()
    shared = manager.shared_live()
    reused = manager.shared_live()
    reusable = manager.reusable_live()
    reusable_again = manager.reusable_live()
    isolated = manager.isolated_live()
    local = manager.local()
    initialized_live = manager.isolated_live("--ports", "auto", deploy=False)
    paths = [
        Path(instance.deployment_dir.name)
        for instance in (
            controlled,
            shared,
            reusable,
            isolated,
            local,
            initialized_live,
        )
    ]

    # Then
    assert reused is shared
    assert reusable_again is reusable
    assert len(set(paths)) == len(paths)
    assert manager.created_count == len(paths)
    assert controlled.has_status(StatusInitialized)
    assert shared.has_status(StatusDatabaseReady)
    assert isolated.has_status(StatusDatabaseReady)
    assert local.has_status(StatusDatabaseReady)
    assert initialized_live.has_status(StatusInitialized)
    assert launcher.init_args[initialized_live.deployment_dir.name][-2:] == (
        "--ports",
        "auto",
    )
    assert launcher.configs[local.deployment_dir.name].infra == "local"
    manager.verify_shared_ready(shared)

    launcher.statuses[shared.deployment_dir.name] = "stopped"
    with pytest.raises(RuntimeError, match="was not left database-ready"):
        manager.verify_shared_ready(shared)

    manager.close()
    assert launcher.destroy_calls == [
        instance.deployment_dir.name
        for instance in (shared, reusable, isolated, local, initialized_live)
    ]
    assert not any(path.exists() for path in paths)


@pytest.mark.parametrize(
    ("deploy_error", "expected_error"),
    [
        (RuntimeError("provisioning failed"), "provisioning failed"),
        (
            TimeoutExpired("deploy", DeploymentManager.DEPLOY_TIMEOUT_SECONDS),
            "Deploy command timed out",
        ),
    ],
)
def test_provisioning_failure_cleans_up_owned_deployment(
    deploy_error: Exception, expected_error: str
) -> None:
    # Given
    launcher = FakeLauncher()
    launcher.deploy_error = deploy_error
    manager = DeploymentManager(
        cast("Launcher", launcher),
        DeploymentConfig(infra="aws", cluster_size=2),
    )

    # When / Then
    with pytest.raises(RuntimeError, match=expected_error):
        manager.isolated_live()

    assert len(launcher.destroy_calls) == 1
    assert not Path(launcher.destroy_calls[0]).exists()
    manager.close()
    assert len(launcher.destroy_calls) == 1


def test_not_ready_deployment_is_cleaned_up() -> None:
    # Given
    launcher = FakeLauncher()
    launcher.ready_after_deploy = False
    manager = DeploymentManager(
        cast("Launcher", launcher),
        DeploymentConfig(infra="aws", cluster_size=2),
    )

    # When / Then
    with pytest.raises(RuntimeError, match="Expected status `database_ready`"):
        manager.isolated_live()

    assert len(launcher.destroy_calls) == 1
    assert not Path(launcher.destroy_calls[0]).exists()


def test_release_destroys_live_deployment_once_only() -> None:
    # Given
    launcher = FakeLauncher()
    manager = DeploymentManager(
        cast("Launcher", launcher),
        DeploymentConfig(infra="aws", cluster_size=2),
    )
    controlled = manager.controlled()
    live = manager.isolated_live()

    # When
    manager.release(controlled)
    manager.release(live)
    manager.release(live)

    # Then
    assert launcher.destroy_calls == [live.deployment_dir.name]


def test_reusable_live_deployment_is_restored_after_mutation(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    launcher = FakeLauncher()
    manager = DeploymentManager(
        cast("Launcher", launcher),
        DeploymentConfig(infra="aws", cluster_size=2),
    )
    deployment = manager.reusable_live()
    launcher.statuses[deployment.deployment_dir.name] = "stopped"
    monkeypatch.setattr(deployment, "db_connectable", lambda: True)

    manager.restore_reusable_ready(deployment)

    assert launcher.start_calls == [deployment.deployment_dir.name]
    assert deployment.has_status(StatusDatabaseReady)
    manager.close()


def test_failed_reusable_restoration_destroys_and_replaces_deployment(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    launcher = FakeLauncher()
    manager = DeploymentManager(
        cast("Launcher", launcher),
        DeploymentConfig(infra="aws", cluster_size=2),
    )
    deployment = manager.reusable_live()
    monkeypatch.setattr(deployment, "db_connectable", lambda: False)

    with pytest.raises(
        RuntimeError, match="restoration failed; deployment was destroyed"
    ):
        manager.restore_reusable_ready(deployment)

    assert launcher.destroy_calls == [deployment.deployment_dir.name]
    assert manager.reusable_live() is not deployment
    manager.close()


def test_failed_reusable_cleanup_quarantines_deployment(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    launcher = FakeLauncher()
    manager = DeploymentManager(
        cast("Launcher", launcher),
        DeploymentConfig(infra="aws", cluster_size=2),
    )
    deployment = manager.reusable_live()
    launcher.destroy_failures = 1
    monkeypatch.setattr(deployment, "db_connectable", lambda: False)

    with pytest.raises(RuntimeError, match="restoration and cleanup failed"):
        manager.restore_reusable_ready(deployment)

    with pytest.raises(RuntimeError, match="is quarantined"):
        manager.reusable_live()

    manager.close()


def test_close_attempts_every_cleanup_and_reports_failures() -> None:
    # Given
    launcher = FakeLauncher()
    manager = DeploymentManager(
        cast("Launcher", launcher),
        DeploymentConfig(infra="aws", cluster_size=2),
    )
    first = manager.isolated_live()
    second = manager.isolated_live()
    first_path = Path(first.deployment_dir.name)
    second_path = Path(second.deployment_dir.name)
    launcher.destroy_failures = 1

    # When / Then
    with pytest.raises(RuntimeError, match="Deployment cleanup failed"):
        manager.close()

    assert launcher.destroy_calls == [str(first_path), str(second_path)]
    assert first_path.exists()
    assert not second_path.exists()

    # When cleanup becomes possible
    manager.close()

    # Then the retained deployment is cleaned up
    assert launcher.destroy_calls[-1] == str(first_path)
    assert not first_path.exists()


@pytest.mark.parametrize("marker_name", ["providers", "platform"])
def test_marker_values_reject_unknown_values(marker_name: str) -> None:
    # Given
    item = cast("pytest.Item", MarkedItem(marker_name, "known", "unknown"))

    # When / Then
    with pytest.raises(pytest.UsageError, match=f"Unknown {marker_name}: unknown"):
        _marker_values(item, marker_name, ("known",))


def test_launcher_area_rejects_live_fixtures() -> None:
    item = cast("pytest.Item", MarkedItem(""))

    with pytest.raises(pytest.UsageError, match="cannot request live fixtures"):
        _validate_launcher_boundary(item, ("isolated_live_deployment",))


def test_only_cloud_live_area_requires_explicit_opt_in() -> None:
    launcher_item = cast("pytest.Item", MarkedItem(""))
    live_item = cast("pytest.Item", MarkedItem("", path="tests/live/test_example.py"))

    assert not _requires_cloud_opt_in(launcher_item, "aws")
    assert not _requires_cloud_opt_in(live_item, "local")
    assert _requires_cloud_opt_in(live_item, "aws")


def test_xdist_groups_follow_deployment_ownership() -> None:
    assert _xdist_group(("deployment_manager",), set(), "local") == "local-live"
    assert _xdist_group((), {"local"}, "local") == "local-live"
    assert _xdist_group(("shared_live_deployment",), set(), "aws") == "shared-live"
    assert _xdist_group(("reusable_live_deployment",), set(), "aws") == "reusable-live"
    assert _xdist_group(("isolated_live_deployment",), set(), "aws") is None
    assert _execution_boundary(("deployment",), set(), "aws") == "controlled"
    assert (
        _execution_boundary(("shared_live_deployment",), set(), "aws") == "shared-live"
    )
    assert (
        _execution_boundary(("reusable_live_deployment",), set(), "aws")
        == "reusable-live"
    )


def test_evidence_summary_reports_selected_and_omitted_targets() -> None:
    evidence = {
        "boundary": {"reusable-live"},
        "provider": {"aws"},
        "platform": {"linux-amd64"},
        "suite": {"smoke"},
    }

    summary = "\n".join(_evidence_summary(evidence, deployments_created=3))

    assert "providers selected: aws" in summary
    assert "execution boundaries selected: reusable-live" in summary
    assert "providers omitted: azure, exoscale, local" in summary
    assert "specialized suites selected: smoke" in summary
    assert "STACKIT omitted" in summary
    assert "deployments created: 3" in summary
