# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

"""Tests for the built-in (official catalog) script language containers."""

import json
import sys
import textwrap
from collections.abc import Iterator
from subprocess import CalledProcessError, CompletedProcess
from typing import Any, Final

import pytest

from framework.deployment import Deployment
from framework.launcher import DeploymentConfig, Launcher

PYTHON_ALIAS: Final = "PYTHON3"

UNKNOWN_ALIAS: Final = "invalid-test-slc-alias"


@pytest.fixture(scope="module")
def slc_deployment(exasol_path: str, infra: str) -> Iterator[Deployment]:
    if infra != "local":
        pytest.skip("SLC is currently supported only on local deployments")

    deployment = Deployment(Launcher(exasol_path), config=DeploymentConfig(infra=infra))
    try:
        deployment.deploy()
        yield deployment
    finally:
        deployment.cleanup()


def _slc(
    deployment: Deployment, *args: str, capture: bool = False
) -> CompletedProcess[str]:
    return deployment.launcher.run_command(
        "slc",
        deployment.deployment_dir.name,
        *args,
        capture_output=capture,
    )


def _slc_statuses(deployment: Deployment) -> list[dict[str, Any]]:
    """Return the parsed `slc list --json` status entries."""
    result = _slc(deployment, "list", "--json", capture=True)
    statuses: list[dict[str, Any]] = json.loads(result.stdout)
    return statuses


def _status_for_alias(deployment: Deployment, alias: str) -> dict[str, Any]:
    """Return the unique catalog entry declaring an SLC alias."""
    matches = [
        status
        for status in _slc_statuses(deployment)
        if any(
            candidate.casefold() == alias.casefold() for candidate in status["aliases"]
        )
    ]
    assert len(matches) == 1, f"expected exactly one catalog SLC for alias {alias!r}"
    return matches[0]


def _is_alias_installed(deployment: Deployment, alias: str) -> bool:
    """Report whether the catalog SLC declaring an alias is marked installed."""
    return bool(_status_for_alias(deployment, alias)["installed"])


def _run_scalar_udf(deployment: Deployment, alias: str, schema: str) -> str:
    """Create and run a trivial scalar UDF; return the connect stdout."""
    script = textwrap.dedent(
        f"""\
        DROP SCHEMA IF EXISTS {schema} CASCADE;
        CREATE SCHEMA {schema};
        OPEN SCHEMA {schema};
        CREATE OR REPLACE {alias} SCALAR SCRIPT hello() RETURNS VARCHAR(10) AS
        def run(ctx):
            return 'hi'
        /
        SELECT hello();
        """
    )
    return deployment.connect(input=script, capture_output=True).stdout


def _assert_database_responds(deployment: Deployment) -> None:
    """Assert that the database remains reachable independently of SLC availability."""
    result = deployment.connect(input="SELECT * FROM Dual", capture_output=True)
    assert "DUMMY" in result.stdout


def _assert_python_udf_is_unavailable(deployment: Deployment, schema: str) -> None:
    """Assert that the database is reachable but Python UDFs are unavailable."""
    _assert_database_responds(deployment)
    assert "hi" not in _run_scalar_udf(deployment, PYTHON_ALIAS, schema)


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
@pytest.mark.installation_e2e
@pytest.mark.local_e2e
def test_slc_list_reports_catalog_containers(slc_deployment: Deployment) -> None:
    """`slc list` reports the catalog in text and JSON, whatever is installed."""
    # When / Then: text listing shows the table and the Python alias.
    text = _slc(slc_deployment, "list", capture=True).stdout
    assert "FLAVOR" in text
    assert PYTHON_ALIAS in text

    # When / Then: JSON listing carries the documented fields for every entry.
    statuses = _slc_statuses(slc_deployment)
    assert statuses, "catalog listing should not be empty on a supported platform"
    required_fields = {"language", "flavor", "version", "aliases", "installed"}
    for status in statuses:
        assert required_fields <= status.keys()
        assert isinstance(status["installed"], bool)

    python = _status_for_alias(slc_deployment, PYTHON_ALIAS)
    assert PYTHON_ALIAS in python["aliases"]


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
@pytest.mark.installation_e2e
@pytest.mark.local_e2e
def test_slc_install_rejects_unknown_alias(slc_deployment: Deployment) -> None:
    """An unknown alias fails before any restart, leaving install state unchanged."""
    # Given: the current install state (order-independent on the shared deployment).
    was_installed = _is_alias_installed(slc_deployment, PYTHON_ALIAS)

    # When: installing an unknown alias.
    with pytest.raises(CalledProcessError) as exc_info:
        _slc(slc_deployment, "install", UNKNOWN_ALIAS, "--auto-approve", capture=True)

    # Then: the error names the failure and valid aliases, and nothing changed.
    stderr = exc_info.value.stderr or ""
    assert "unknown SLC alias" in stderr
    assert PYTHON_ALIAS in stderr
    assert _is_alias_installed(slc_deployment, PYTHON_ALIAS) == was_installed


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
@pytest.mark.installation_e2e
@pytest.mark.local_e2e
def test_slc_remove_when_not_installed_is_noop(slc_deployment: Deployment) -> None:
    """Removing an SLC that is not installed succeeds as a no-op without restarting."""
    # Given: R is never installed by this suite (so the outcome is order-independent).
    r_alias: Final = "R"

    # When / Then: removing it succeeds and reports nothing to remove.
    result = _slc(slc_deployment, "remove", r_alias, capture=True)
    assert "nothing to remove" in result.stdout


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
@pytest.mark.installation_e2e
@pytest.mark.local_e2e
def test_official_slc_install_runs_udf(slc_deployment: Deployment) -> None:
    """Installing an official SLC makes its UDFs runnable; reinstalling is a no-op."""
    # When / Then: installing marks it installed and a Python UDF runs.
    _slc(slc_deployment, "install", PYTHON_ALIAS, "--auto-approve")
    assert _is_alias_installed(slc_deployment, PYTHON_ALIAS)
    assert "hi" in _run_scalar_udf(slc_deployment, PYTHON_ALIAS, "slc_e2e_official")

    # When / Then: reinstalling the same alias is idempotent, without a restart.
    reinstall = _slc(
        slc_deployment, "install", PYTHON_ALIAS, "--auto-approve", capture=True
    )
    assert "already installed and up to date" in reinstall.stdout
    assert _is_alias_installed(slc_deployment, PYTHON_ALIAS)


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
@pytest.mark.installation_e2e
@pytest.mark.local_e2e
def test_official_slc_remove_uninstalls_language(slc_deployment: Deployment) -> None:
    """Removing an installed official SLC clears its status and makes its UDFs fail."""
    # Given: the Python SLC is installed (idempotent setup) and its UDFs run.
    _slc(slc_deployment, "install", PYTHON_ALIAS, "--auto-approve")
    assert _is_alias_installed(slc_deployment, PYTHON_ALIAS)
    assert "hi" in _run_scalar_udf(slc_deployment, PYTHON_ALIAS, "slc_e2e_remove")

    # When: removing it (restarts the database, unmounting the language).
    _slc(slc_deployment, "remove", PYTHON_ALIAS, "--auto-approve")

    # Then: it is no longer installed and its UDFs can no longer run.
    assert not _is_alias_installed(slc_deployment, PYTHON_ALIAS)
    _assert_python_udf_is_unavailable(slc_deployment, "slc_e2e_remove")


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
@pytest.mark.installation_e2e
@pytest.mark.local_e2e
def test_slc_install_no_restart_activates_on_next_start(
    slc_deployment: Deployment,
) -> None:
    """`--no-restart` records the SLC without applying it; the next start mounts it."""
    # Given: the Python SLC is neither recorded nor mounted (database left running).
    _slc(slc_deployment, "remove", PYTHON_ALIAS, "--auto-approve")

    # When: installing with --no-restart against the running database.
    result = _slc(slc_deployment, "install", PYTHON_ALIAS, "--no-restart", capture=True)

    # Then: it is recorded as installed but deferred, and not yet usable.
    assert "next start" in result.stdout
    assert _is_alias_installed(slc_deployment, PYTHON_ALIAS)
    _assert_python_udf_is_unavailable(slc_deployment, "slc_e2e_defer")

    # When / Then: a restart applies the recorded state, mounting the SLC.
    slc_deployment.stop()
    slc_deployment.start()
    assert "hi" in _run_scalar_udf(slc_deployment, PYTHON_ALIAS, "slc_e2e_defer")


@pytest.mark.skipif(
    sys.platform.startswith("win"), reason="Test is not supported on Windows OS"
)
@pytest.mark.installation_e2e
@pytest.mark.local_e2e
def test_slc_update_when_current_is_noop(slc_deployment: Deployment) -> None:
    """Updating an SLC already at the catalog version is a no-op and stays usable."""
    # Given: the Python SLC is installed at the catalog version.
    _slc(slc_deployment, "install", PYTHON_ALIAS, "--auto-approve")
    assert _is_alias_installed(slc_deployment, PYTHON_ALIAS)

    # When / Then: updating reports no change and the language is still usable.
    result = _slc(
        slc_deployment, "update", PYTHON_ALIAS, "--auto-approve", capture=True
    )
    assert "already up to date" in result.stdout
    assert _is_alias_installed(slc_deployment, PYTHON_ALIAS)
    assert "hi" in _run_scalar_udf(slc_deployment, PYTHON_ALIAS, "slc_e2e_update")
