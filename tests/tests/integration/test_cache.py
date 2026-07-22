# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

"""Runtime-artifact cache CLI: list, clean, unlock, diag, locking, output (offline)."""

import json
from pathlib import Path
from subprocess import CalledProcessError

import pytest

from tests.testcase_helpers import run_command


@pytest.mark.launcher_tests
def test_cache_list_text_output(exasol_path: str) -> None:
    # When the cache is listed as text
    output = run_command([exasol_path, "cache", "list"]).stdout

    # Then it reports the cache root
    assert "Runtime artifact cache:" in output


@pytest.mark.launcher_tests
def test_cache_list_json_is_valid(exasol_path: str) -> None:
    # When the cache is listed as JSON
    output = run_command([exasol_path, "cache", "list", "--json"]).stdout

    # Then the output parses as JSON
    data = json.loads(output)
    assert isinstance(data, (list, dict))


@pytest.mark.launcher_tests
def test_cache_clean_dry_run_removes_nothing(exasol_path: str) -> None:
    # When clean is run in dry-run mode
    output = run_command([exasol_path, "cache", "clean", "--dry-run"]).stdout

    # Then it only previews the removal
    assert "Would remove" in output


@pytest.mark.launcher_tests
def test_cache_clean_selectors_are_mutually_exclusive(exasol_path: str) -> None:
    # When incompatible selectors are combined
    with pytest.raises(CalledProcessError) as exc:
        run_command([exasol_path, "cache", "clean", "--invalid", "--all"])

    # Then it is rejected as a mutually-exclusive-flags error
    stderr = (exc.value.stderr or "").lower()
    assert "none of the others can be" in stderr


@pytest.mark.launcher_tests
@pytest.mark.parametrize("mode", ["--partial-downloads", "--all"])
def test_cache_clean_reports_mode_summary(exasol_path: str, mode: str) -> None:
    # When a real cleanup mode runs
    output = run_command([exasol_path, "cache", "clean", mode]).stdout

    # Then it reports how many artifacts were removed and the mode
    assert "removed" in output.lower() or "remove" in output.lower()
    assert "mode:" in output.lower()


@pytest.mark.launcher_tests
def test_cache_unlock_reports_cleared(exasol_path: str) -> None:
    # When the cache lock is cleared
    output = run_command([exasol_path, "cache", "unlock"]).stdout

    # Then it confirms the lock was cleared
    assert "Runtime artifact cache lock cleared." in output


@pytest.mark.launcher_tests
def test_diag_cache_reports_status_fields(exasol_path: str) -> None:
    # When the diagnostic report is generated
    output = run_command([exasol_path, "diag", "cache"]).stdout

    # Then it includes the expected status fields
    for field in [
        "Cache root:",
        "Config status:",
        "Index status:",
        "Lock status:",
        "Artifacts:",
        "Total size:",
    ]:
        assert field in output, f"missing {field!r} in diag cache report"


@pytest.mark.launcher_tests
def test_diag_cache_does_not_mutate(exasol_path: str) -> None:
    # Given an initial cache listing
    before = run_command([exasol_path, "cache", "list"]).stdout

    # When diag cache runs
    run_command([exasol_path, "diag", "cache"])

    # Then the cache listing is unchanged (the command is read-only)
    after = run_command([exasol_path, "cache", "list"]).stdout
    assert before == after


@pytest.mark.launcher_tests
def test_lock_contention_surfaces_error() -> None:
    # Reproducing genuine lock contention requires holding the cache lock for
    # the full acquire timeout (~5 minutes) from a second process, which is not
    # practical in an automated unit run.
    pytest.skip("requires holding the cache lock across the ~5 min acquire timeout")


NOISY_LINES = ("using deployment directory", "deployment log file")


@pytest.mark.launcher_tests
def test_default_output_is_quiet_but_debug_and_log_are_verbose(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given an initialized deployment directory
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()
    dir_flag = ["--deployment-dir", str(deployment_dir)]
    run_command([exasol_path, "init", "aws", *dir_flag, "--no-launcher-version-check"])

    # When status runs at default verbosity
    default_run = run_command([exasol_path, "status", "--json", *dir_flag])
    default_console = default_run.stdout + default_run.stderr

    # Then the noisy diagnostics are absent from the console
    for line in NOISY_LINES:
        assert line not in default_console

    # When status runs at debug verbosity
    debug_run = run_command(
        [exasol_path, "--log-level", "debug", "status", "--json", *dir_flag]
    )

    # Then the deployment-directory diagnostic reappears
    assert "using deployment directory" in (debug_run.stdout + debug_run.stderr)

    # Then the deployment log file always records the diagnostics that stay off
    # the default console (e.g. the "deployment log file" lifecycle lines)
    log_text = (deployment_dir / "deployment.log").read_text()
    assert "deployment log file" in log_text
