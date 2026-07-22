

# Deployment Testing

Deployment tests can be triggered using `task github:trigger-deployment-tests`

`tests/tests/deployment/test_standard_deployment.py`

| Test | Targets | Description |
|------|---------|-------------|
| `test_single_query` | local, cloud | Runs a basic `SELECT * FROM Dual` query to confirm the DB is reachable and functional |
| `test_exit_command` | local, cloud | Confirms the `exit` command properly terminates the interactive shell session |
| `test_multiple_queries` | local, cloud | Runs a sequence of DDL + DML operations (create schema, create table, insert rows, select) |
| `test_file_import` | local, cloud | Tests CSV file import into a table |
| `test_connect_table_width` | local, cloud | Validates query output formatting with varying column widths and long content |
| `test_connect_interactive_shows_version_and_exit_hint` | local, cloud | Verifies that interactive mode displays the Exasol version string and an `exit` hint on startup |
| `test_diag_cos_runs_confd_client` | cloud | Runs COS diagnostic commands (skipped for local infra, which uses a VM shell fallback) |
| `test_license_session_limit` | local, cloud | Confirms the Exasol Personal license enforces a 20 concurrent session cap |

`tests/tests/deployment/test_local_deployment.py`

| Test | Targets | Description |
|------|---------|-------------|
| `test_ports_override_sets_db_port` | local | Verifies that `--ports db:<port>` correctly routes the DB port through to the VM; confirms the DB is reachable on the custom port |
| `test_ports_override_stable_across_restarts` | local | Verifies that a custom DB port assigned at `exasol init` is preserved in `deployment.json` and remains reachable after a stop/start cycle |

# Integration Testing

Integration tests can be run using `task tests-integration` or in CI using `task github:trigger-integration-tests`. They run on Ubuntu and Windows in CI.

## CLI

`tests/tests/integration/test_cli.py`

| Test | Description |
|------|-------------|
| `test_help_flag` | Tests that the `--help` flag outputs correct help and usage information |
| `test_version` | Verifies the version command outputs the git tag version correctly |
| `test_version_json` | Tests version command with JSON output format |
| `test_info_command_exists` | Verifies the info command is available and shows proper help |
| `test_info_reports_missing_deployment_without_error` | Tests info command behavior when no deployment exists |
| `test_info_json_reports_missing_deployment_without_error` | Tests JSON output of info command when deployment is missing |
| `test_info_command_init_deployment` | Tests info command output after initialization |

## Presets

`tests/tests/integration/test_presets.py`

| Test | Description |
|------|-------------|
| `test_presets_help_mentions_subcommands` | Verifies the presets command shows list and export subcommands |
| `test_presets_list_outputs_sections` | Tests that presets list shows infrastructure and installation headers |
| `test_presets_list_json_is_valid` | Tests presets list with JSON output format |
| `test_presets_export_writes_files` | Tests exporting infrastructure and installation presets to a directory |
| `test_presets_export_fails_on_non_empty_dir` | Tests that export fails when the target directory is not empty |

## Deployment Directory Resolution

`tests/tests/integration/test_deployment_directory_resolution.py`

| Test | Description |
|------|-------------|
| `test_status_uses_default_deployment_dir_without_corrupting_json` | Tests default deployment directory resolution with JSON output |
| `test_status_reports_uninitialized_explicit_deployment_dir` | Tests explicit deployment directory handling |
| `test_status_debug_logs_current_deployment_dir` | Tests debug logging of the current deployment directory |
| `test_init_creates_default_deployment_dir` | Tests that init automatically creates the default deployment directory |
| `test_info_reports_uninitialized_resolved_default_dir` | Tests info command output with a resolved default directory |

## Init

`tests/tests/integration/test_init.py`

| Test | Description |
|------|-------------|
| `test_init_defaults_and_help` | Tests that init help displays preset names and references |
| `test_init_requires_infra_preset_arg` | Tests that init requires an infrastructure preset argument |
| `test_init_succeeds` | Tests successful init including EULA display |
| `test_init_creates_deployment_dir` | Tests that init creates the deployment directory |
| `test_init_allows_deployment_dir_flag_before_preset_arg` | Tests that the deployment dir flag can appear before the preset argument |
| `test_init_fails_on_non_empty_directory` | Tests that init rejects a non-empty target directory |
| `test_init_idempotent` | Tests that init can be re-run without modifying an already-initialized deployment |
| `test_init_accepts_infra_preset_path` | Tests init with an exported preset directory path |
| `test_init_accepts_install_preset_path_as_second_arg` | Tests passing an installation preset as a second argument |
| `test_init_performs_version_check` | Tests that init performs a launcher version check |
| `test_init_skips_version_check` | Tests that `--no-launcher-version-check` suppresses the version check |

## Version Check

`tests/tests/integration/test_version_check.py`

| Test | Description |
|------|-------------|
| `test_version_check_latest` | Tests version check with formatted output |
| `test_version_check_latest_json` | Tests version check with JSON output |
| `test_version_check_latest_when_up_to_date` | Tests version check behavior when the current version matches the latest |

## Install

`tests/tests/integration/test_install.py`

| Test | Description |
|------|-------------|
| `test_install_requires_infra_preset_arg` | Tests that the install command requires a preset argument |
| `test_install_help` | Tests install help documentation |
| `test_install_executes_init_step` | Tests that install executes the init step and surfaces failures correctly |
| `test_init_local_rejects_unsupported_platform_before_writing_files` | Tests platform validation for local deployments |
| `test_init_local_accepts_explicit_minimum_memory` | Tests minimum memory configuration for local deployments |
| `test_init_local_rejects_memory_below_minimum` | Tests that memory below the minimum is rejected before any files are written |
| `test_deploy_local_with_fake_runner_override` | Tests local deployment end-to-end using a fake runner script |

## Reconfiguration

`tests/tests/integration/test_reconfiguration.py`

| Test | Description |
|------|-------------|
| `test_config_set_updates_same_preset_variables` | Tests that config set preserves infrastructure state when updating variables |
| `test_init_updates_same_preset_variables` | Tests re-running init with changed options |
| `test_config_get_outputs_active_configuration` | Tests querying the active configuration as JSON |
| `test_config_reset_restores_selected_defaults` | Tests resetting individual configuration options to their defaults |
| `test_config_reset_all_restores_all_defaults` | Tests resetting all options to their defaults |
| `test_config_set_refuses_running_deployment` | Tests that config set is rejected while a deployment is running |
| `test_config_set_refuses_state_with_possible_resources` | Tests config set rejection for failed/interrupted deployment states |
| `test_install_updates_same_preset_configuration_before_retry` | Tests that install preserves state while applying configuration updates on retry |
| `test_install_refuses_same_preset_configuration_change_for_running_deployment` | Tests that install refuses configuration changes to a running deployment |
| `test_init_refuses_different_preset_without_remove` | Tests that init rejects switching presets without first removing the existing deployment |
| `test_install_refuses_different_preset_without_removing_local_state` | Tests that install validates preset switching and rejects without removal |
| `test_destroy_remove_removes_local_deployment_directory` | Tests destroy with local state cleanup |
| `test_remove_removes_local_deployment_directory_without_destroy` | Tests removing a deployment without destroying cloud resources |
| `test_remove_refuses_non_deployment_directory` | Tests that remove validates the target is a deployment directory |
| `test_install_retries_same_preset_after_failed_state` | Tests install retry with preserved state after a failure |