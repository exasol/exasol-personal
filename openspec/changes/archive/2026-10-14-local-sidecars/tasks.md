## 1. Running sidecars on a local deployment

- [x] 1.1 Add a shared Podman client over the execution environment, used by
  both the database install and sidecars, that invokes podman, captures its
  streams and carries resolved environment values to containers through an
  owner-readable file removed once the container is created. Reject values the
  file format cannot carry, naming the variable. Add Podman sidecar creation,
  inspection, restart-policy translation, definition comparison, and
  owned-resource cleanup on that client, keying the definition label with the
  deployment salt. Provide
  one DNS-enabled deployment bridge with `com.exasol.launcher` ownership
  labels, attaching an existing bridge-backed database and reporting the
  explicit restart requirement otherwise. Carry a failed start's short cause
  and full diagnostic separately. Verify command contracts, database
  attachment, DNS aliases, publication, image defaults, restart policies,
  startup diagnostics, and cleanup with `task tests-unit`.
- [x] 1.2 Give the deployment backend a sidecar capability that applies a
  desired set and reports observations, and orchestrate enable, disable,
  status, deploy, and already-running start against it under the deployment
  lock. Report sidecars as unsupported for backends that do not implement it.
  Preserve saved intent and report sidecar errors separately from database
  outcomes. Keep `sidecars-state.json` advisory and separate recorded
  diagnostics from live errors. Offer the status call to action when a sidecar
  fails to start. Verify the capability contract, the unsupported path,
  lifecycle ordering, stopped enablement, catalog-independent saved
  operations, edited definitions, interrupted cleanup, complete, pending and
  unknown outcomes, runtime-preserving inspection, and user-edited-name
  cleanup retries with `task tests-unit` and `task tests-launcher`.

## 2. Platform publication, validation, and documentation

The macOS integration uses the companion runner's live-forwarding commands.
Development validation can use an embedded-resource override with the
companion build. Release validation uses the compatible released runner.

- [x] 2.1 Adapt macOS live forwarding to the sidecar capability, publishing
  declared host ports into the VM guest and withdrawing forwards it no longer
  serves. Verify forwarding add and remove, withdrawal of unserved and
  orphaned endpoints, endpoint reporting, and runtime cleanup with
  `task tests-unit`.
- [x] 2.2 Validate the lifecycle on a real local deployment with the pinned
  Caddy fixture definition, covering connection values, publication, service
  DNS, restart policy, reconciliation, recovery, and cleanup. Verify
  `task tests-deployment-local`.
- [x] 2.3 Document sidecar user workflows, supported configuration, and local
  networking. Add navigation and verify `task docs-check` and
  `task docs-build`.
- [x] 2.4 Document catalog extension and backend sidecar integration in the
  contributor guide. Link the architecture, user configuration, and fixture
  testing guidance. Verify source contracts, relative links, and Markdown
  formatting.

## Commit and completion boundaries

Each numbered task is one independently verifiable commit. Keep updates to
existing behavior documentation with their implementation. Commit new
documentation separately. Run the full relevant checks for each commit. Keep
all files in this active change uncommitted and update checkboxes locally as
work completes.

Section 1 forms the first pull request, which runs sidecars on deployments
whose containers bind directly. Section 2 forms the second, which adds macOS
publication, the live validation covering both, and the documentation.

CI owns release-runner compatibility, Windows publication, and full local
deployment validation using the pinned Caddy fixture definition.

After all implementation tasks pass, sync the delta specs, archive this change,
and make the pure OpenSpec result the final branch commit. Preserve the user's
one-line Conventional Commit format.

## Scenario-to-test ownership

Sidecar lifecycle and networking scenarios are ADDED. Each has exactly one
owning test, identified for review with the `openspec` marker on live tests.
Parameterization and supported-platform executions remain instances of that
test.

### sidecar-networking

- Database and sidecar communicate through service names:
  `test_sidecar_bidirectional_service_dns` (live).
- Separate deployments reuse service aliases:
  `test_sidecar_deployment_dns_scope` (live).
- Published endpoint is reachable from the local host:
  `test_sidecar_loopback_endpoint` (live).
- Internal service port stays internal: `test_sidecar_internal_port` (live).
- Broad publication request receives a validation error:
  `TestSidecarLoopbackPublicationValidation` (configuration unit).
- Enabling joins an existing running database:
  `test_sidecar_existing_database_network` (live).
- Existing non-bridge deployment waits for explicit restart:
  `test_sidecar_legacy_network_restart` (live).

### sidecar-lifecycle

- Unsupported deployment reports the limitation:
  `test_sidecar_unsupported_deployment_type` (launcher).
- Enable on a running deployment is idempotent:
  `test_sidecar_enable_idempotent` (live).
- Enable waits for the deployment to start:
  `test_sidecar_deferred_enable` (live, parameterized by initial state).
- Start applies an edited definition:
  `test_sidecar_start_reconciles_marker` (live).
- Start applies rotated connection values:
  `test_sidecar_rotated_credentials` (live).
- Repeated start retains an unchanged sidecar:
  `test_sidecar_start_preserves_container` (live).
- Start completes an interrupted sidecar operation:
  `test_sidecar_interrupted_enable_recovery` (live, chaos).
- Start removes a definition deleted by the user:
  `test_sidecar_start_removes_deleted_definition` (live).
- Deployment restart restores enabled sidecars:
  `test_sidecar_deployment_restart` (live).
- Process exit follows the selected policy:
  `test_sidecar_process_restart_policy` (live, parameterized).
- Disable on a stopped deployment leaves it stopped:
  `TestSidecarDisableDefersPublicationWhileStopped` (deployment unit).
- Repeated disable retains database availability:
  `test_sidecar_disable_idempotent` (live).
- Disable honors user-edited names:
  `TestSidecarDisableUsesEditedNames` (deployment unit).
- Unrepresentable value is rejected:
  `TestSidecarRejectsUnrepresentableValue` (runtime unit, parameterized by
  line break).
- Failed startup offers the status call to action:
  `TestSidecarStartupFailureOffersStatus` (command unit, parameterized by
  cause).
- Status reports running and stopped sidecars:
  `test_sidecar_status_lifecycle` (live).
- Sidecar failure is reported separately:
  `test_sidecar_failure_preserves_database_status` (live).
- Status refreshes resolved failures:
  `TestSidecarStatusRefreshesRecordedFailures` (deployment unit, parameterized).
- Destroy removes deployment service resources:
  `test_sidecar_destroy_cleanup` (live).

### Fixture and execution details

Live evidence lives beside its capability in `tests/live/test_sidecar.py` and
annotates recovery evidence with `chaos`. Tests save the fixture definition
before enabling it, so they run against the ordinary launcher build. The
deployment manager owns one local deployment at a time, so deployment-scoped
service names use directly owned instances. Use a controlled process exit
inside the container for restart testing. Three supplementary live tests carry
no scenario of their own: `test_sidecar_status_reads_runtime_observations`
verifies the status-refresh contract through real Podman and local runtime
adapters, `test_sidecar_environment_values` verifies that resolved connection
values reach the container without being saved, and
`test_sidecar_edited_definition_drops_password` verifies that removing a
reference withdraws its value.
