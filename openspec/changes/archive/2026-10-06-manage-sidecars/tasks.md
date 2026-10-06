## 1. Configuration

- [x] 1.1 Add the embedded catalog, typed Container subset, strict validation,
  architecture resolution, and catalog injection seam. Verify catalog lookup,
  defaults, reserved names, and invalid-field tests with `task tests-unit`.
- [x] 1.2 Add atomic deployment definition storage and catalog-to-deployment
  materialization under the deployment lock. Verify first enable, preserved
  edits and empty-file behavior with `task tests-unit`.
- [x] 1.3 Add connection-reference resolution, password opt-out, Kubernetes
  expansion, and redacted runtime configuration delivery through optional
  child-process environment overrides on `Run`. Verify required and optional
  references, expansion, environment delivery, preserved command input and
  arguments, and diagnostics tests with `task tests-unit`.

## 2. Runtime support

The macOS integration uses the companion runner's live-forwarding commands.
Development validation can use an embedded-resource override with the
companion build. Release validation uses the compatible released runner.

- [x] 2.1 Add shared Podman sidecar creation, inspection, restart-policy
  translation, definition comparison, and owned-resource cleanup through the
  execution environment. Use deployment bridge networks locally and host
  networking in cloud, with `com.exasol.launcher` ownership labels. Verify
  command contracts, database attachment, DNS aliases, publication, image
  defaults, restart policies, and cleanup with `task tests-unit`.
- [x] 2.2 Adapt local runtime providers and macOS live forwarding to the
  shared Podman manager. Verify forwarding add/remove, endpoint reporting,
  and runtime cleanup with `task tests-unit`.
- [x] 2.3 Support Linux local host shells and explicit host commands, preserving
  argument boundaries, environment, working directory, streams, and failures.
  Document shell selection and Linux usage. Verify runtime and CLI tests with
  `task tests-unit` and `task tests-integration`, regenerate the CLI reference,
  and run formatting, linting, and documentation checks. Commit independently.

## 3. Deployment lifecycle

- [x] 3.1 Provide backend sidecar hosts and shared local/cloud orchestration.
  Apply definitions to every cloud node through private SSH Podman execution,
  use host networking in cloud, and report per-host status and failures.
  Encapsulate shared lifecycle hooks for hosts ready, before stop, and after
  destroy. Integrate enable, disable, status, deploy, and already-running start.
  Preserve saved intent and report sidecar errors separately from database
  outcomes. Verify multi-host recovery, lifecycle ordering, architecture
  selection, stopped enablement, catalog-independent saved operations, edited
  definitions, and interrupted cleanup with `task tests-unit`. Keep
  `sidecars-state.json` advisory, separate recorded
  diagnostics from live errors, and resolve saved names independently of the
  catalog. Refresh recorded failures under the exclusive lock when status
  verifies desired state, including applied definitions and publication.
  Verify complete, pending, and unknown outcomes, retained cleanup failures,
  runtime-preserving inspection, and user-edited-name cleanup retries.

## 4. CLI and documentation

- [x] 4.1 Wire `sidecar list`, `enable`, `status`, and `disable`, including
  deployment selection, per-host status, `--json`, and `--no-db-password`.
  Route recovery guidance through the CTA helper, retain factual errors and
  recorded diagnostics, and register deployment-file logging for actions.
  Add command tests and the unreleased changelog entry.
  Regenerate the CLI reference with `task docs-cli-reference`. Verify
  `task tests-integration`, `task docs-check`, and `task docs-build`.

- [x] 4.2 Document sidecar user workflows, supported configuration, and local
  and cloud networking. Add navigation and verify `task docs-check` and
  `task docs-build`. Commit the new user guide separately.

- [x] 4.3 Document catalog extension and host-adapter integration in the
  contributor guide. Link the architecture, user configuration, and fixture
  testing guidance. Verify source contracts, relative links, and Markdown
  formatting. Commit new developer documentation separately.

## Commit and completion boundaries

Each numbered task is one independently verifiable commit. Keep updates to
existing behavior documentation with their implementation. Commit new
documentation separately. Run the full relevant checks for each
commit. Keep all files in this active change uncommitted and update checkboxes
locally as work completes. External workflow dispatch requires confirmation.

CI owns release-runner compatibility, Windows publication, and full local and
cloud deployment validation using the pinned Caddy fixture catalog.

After all implementation tasks pass, sync the delta specs, archive this change,
and make the pure OpenSpec result the final branch commit. Preserve the user's
one-line Conventional Commit format.

## Scenario-to-test ownership

Sidecar scenarios below are ADDED. Linux shell scenarios are MODIFIED.
Each has exactly one planned owning test.
Parameterization and supported-platform executions remain instances of that
test. Additional component tests exercise supplementary implementation cases.
Use Given/When/Then structure, with scenario identity conveyed by test names.

### sidecar-configuration

- Catalog listing describes deployment availability:
  `test_sidecar_list_reports_availability` (CLI integration).
- Unsupported selection explains the available choices:
  `test_sidecar_enable_reports_choices` (CLI integration, parameterized).
- First enable materializes the template:
  `TestSidecarEnableMaterializesTemplate` (deployment unit).
- Repeated enable preserves deployment edits:
  `TestSidecarEnablePreservesEdits` (deployment unit).
- Catalog changes preserve enabled definitions:
  `TestSidecarSavedDefinitionSurvivesCatalogChange` (deployment unit).
- Supported configuration determines container execution:
  `TestSidecarSupportedExecutionSettings` (runtime contract unit).
- Invalid configuration identifies the offending location:
  `TestSidecarInvalidConfigurationLocation` (configuration unit, parameterized).
- Connection values and literal environment reach the container:
  `test_sidecar_environment_values` (deployment, Caddy response and container
  environment).
- Required connection reference is unavailable:
  `TestSidecarRequiredReferenceError` (resolver unit).
- Optional connection reference is unavailable:
  `TestSidecarOptionalReferenceAbsent` (runtime contract unit).
- Password opt-out persists through restart:
  `test_sidecar_password_opt_out` (deployment).
- Edited password opt-out is honored:
  `test_sidecar_edited_password_opt_out` (deployment).
- Expansion preserves Kubernetes escaping and ordering:
  `TestSidecarKubernetesExpansion` (resolver unit, parameterized).
- Failure diagnostics redact injected credentials:
  `TestSidecarCredentialProtection` (runtime contract unit).

### sidecar-networking

- Database and sidecar communicate through service names:
  `test_sidecar_bidirectional_service_dns` (deployment).
- Separate deployments reuse service aliases:
  `test_sidecar_deployment_dns_scope` (deployment).
- Published endpoint is reachable from the local host:
  `test_sidecar_loopback_endpoint` (deployment).
- Internal service port stays internal:
  `test_sidecar_internal_port` (deployment).
- Broad publication request receives a validation error:
  `TestSidecarLoopbackPublicationValidation` (configuration unit).
- Enabling joins an existing running database:
  `test_sidecar_existing_database_network` (deployment).
- Existing non-bridge deployment waits for explicit restart:
  `test_sidecar_legacy_network_restart` (deployment).
- Cloud container receives host networking and database name:
  `TestSidecarCloudHostNetworking` (runtime contract unit).
- Cloud remapping receives a validation error:
  `TestSidecarCloudRejectsRemappingBeforeRuntimeChanges` (runtime contract unit).

### sidecar-lifecycle

- Enable on a running deployment is idempotent:
  `test_sidecar_enable_idempotent` (deployment).
- Enable waits for the deployment to start:
  `test_sidecar_deferred_enable` (deployment, parameterized by initial state).
- Start applies an edited definition:
  `test_sidecar_start_reconciles_marker` (deployment).
- Repeated start retains an unchanged sidecar:
  `test_sidecar_start_preserves_container` (deployment).
- Start completes an interrupted sidecar operation:
  `test_sidecar_interrupted_enable_recovery` (chaos).
- Start removes a definition deleted by the user:
  `test_sidecar_start_removes_deleted_definition` (deployment).
- Deployment restart restores enabled sidecars:
  `test_sidecar_deployment_restart` (deployment).
- Process exit follows the selected policy:
  `test_sidecar_process_restart_policy` (deployment, parameterized).
- Repeated disable retains database availability:
  `test_sidecar_disable_idempotent` (deployment).
- Disable honors user-edited names:
  `TestSidecarDisableUsesEditedNames` (deployment unit).
- Status reports running and stopped sidecars:
  `test_sidecar_status_lifecycle` (deployment).
- Sidecar failure is reported separately:
  `test_sidecar_failure_preserves_database_status` (deployment).
- Status refreshes resolved failures:
  `TestSidecarStatusRefreshesRecordedFailures` (deployment unit, parameterized).
- Destroy removes deployment service resources:
  `test_sidecar_destroy_cleanup` (deployment).
- Cloud enablement precedes provisioning:
  `test_sidecar_cloud_enable_before_provisioning` (CLI integration).
- Hosts reconcile independently after a failure:
  `TestSidecarEveryHostReconcilesIndependently` (deployment unit).
- Disable retries a failed host independently:
  `TestSidecarMultiHostDisableRetriesFailedHost` (deployment unit).

### exasol-local-deployment

- Host shell requested on Linux: `test_linux_host_shell` (CLI integration,
  parameterized by configured shell and fallback).
- Explicit host command requested on Linux: `test_linux_host_command`
  (CLI integration, parameterized by process success and failure).
- Container shell requested on Linux:
  `TestLocalBackend_LinuxContainerShellIsUnsupported` (backend unit).

### Fixture and execution details

Use existing `tests/framework` deployment and cleanup helpers. Place CLI tests
in `tests/tests/integration`, deployment tests in `tests/tests/deployment`, and
interruption tests in `tests/tests/chaos`. Parameterize image policies and exit
policies where needed. Verify both image architectures before recording the
test image digest. Fixture catalog selection is limited to test builds.

Go unit tests isolate deployment and runtime dependencies. Python deployment
tests provision real infrastructure. The supplementary
`test_sidecar_cloud_nodes_reconcile` uses `reusable_deployment` to exercise
the cloud host-networking and shared-definition contracts on every real node,
including environment delivery, edited definitions, missing-container recovery,
and disable cleanup. Cloud HTTP probes execute in the selected node's network
context. The service binds to loopback with equal host and container ports.
The supplementary `test_sidecar_status_reads_runtime_observations` verifies
the status-refresh contract through real Podman and local runtime adapters.

Caddy's HTTP body contains all four injected connection values and a literal
marker. Use only disposable test credentials. Check variable presence through
runtime execution for opt-out scenarios. The DNS-scope test uses internal-only
sidecars so fixed host ports can be reused across independent deployments.

Use a controlled process exit inside the container for restart testing and
observe runtime restart state. Successful explicit lifecycle stops are checked
by the deployment restart and disable tests. Existing database lifecycle and
UDF tests supply regression coverage for network changes.
