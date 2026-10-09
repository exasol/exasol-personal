## 1. Runtime support

The macOS integration uses the companion runner's live-forwarding commands.
Development validation can use an embedded-resource override with the
companion build. Release validation uses the compatible released runner.

- [x] 1.1 Add shared Podman sidecar creation, inspection, restart-policy
  translation, definition comparison, and owned-resource cleanup through the
  execution environment. Use one DNS-enabled deployment bridge with
  `com.exasol.launcher` ownership labels, attaching an existing bridge-backed
  database and reporting the explicit restart requirement otherwise. Verify
  command contracts, database attachment, DNS aliases, publication, image
  defaults, restart policies, and cleanup with `task tests-unit`.
- [x] 1.2 Adapt local runtime providers and macOS live forwarding to the
  shared Podman manager, answering the port-publication contract so the
  deployment opens and reports host endpoints. Verify forwarding add/remove,
  withdrawal of unserved and orphaned endpoints, endpoint reporting, and
  runtime cleanup with `task tests-unit`.

## 2. Deployment lifecycle

- [x] 2.1 Provide backend sidecar hosts, endpoint opening, and shared
  orchestration. Encapsulate lifecycle hooks for hosts ready, before stop, and
  after destroy, and integrate enable, disable, status, deploy, and
  already-running start. Preserve saved intent and report sidecar errors
  separately from database outcomes. Keep `sidecars-state.json` advisory and
  separate recorded diagnostics from live errors. Refresh recorded failures
  under the exclusive lock when status verifies desired state, including
  applied definitions and the endpoints the deployment serves. Verify
  multi-host recovery, lifecycle ordering, stopped enablement,
  catalog-independent saved operations, edited definitions, interrupted
  cleanup, complete, pending and unknown outcomes, opened and direct
  publication checks, runtime-preserving inspection, and user-edited-name
  cleanup retries with `task tests-unit`.

## 3. Validation and documentation

- [x] 3.1 Validate the lifecycle on a real local deployment with the pinned
  Caddy fixture definition, covering connection values, publication, service
  DNS, restart policy, reconciliation, recovery, and cleanup. Verify
  `task tests-deployment-local`.
- [x] 3.2 Document sidecar user workflows, supported configuration, and local
  networking. Add navigation and verify `task docs-check` and
  `task docs-build`. Commit the new user guide separately.
- [x] 3.3 Document catalog extension and host-adapter integration in the
  contributor guide. Link the architecture, user configuration, and fixture
  testing guidance. Verify source contracts, relative links, and Markdown
  formatting. Commit new developer documentation separately.

## Commit and completion boundaries

Each numbered task is one independently verifiable commit. Keep updates to
existing behavior documentation with their implementation. Commit new
documentation separately. Run the full relevant checks for each commit. Keep
all files in this active change uncommitted and update checkboxes locally as
work completes.

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

- Enable on a running deployment is idempotent:
  `test_sidecar_enable_idempotent` (live).
- Enable waits for the deployment to start:
  `test_sidecar_deferred_enable` (live, parameterized by initial state).
- Start applies an edited definition:
  `test_sidecar_start_reconciles_marker` (live).
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
- Repeated disable retains database availability:
  `test_sidecar_disable_idempotent` (live).
- Disable honors user-edited names:
  `TestSidecarDisableUsesEditedNames` (deployment unit).
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
inside the container for restart testing. The supplementary
`test_sidecar_status_reads_runtime_observations` verifies the status-refresh
contract through real Podman and local runtime adapters. Supplementary
multi-host unit tests exercise independent reconciliation and cleanup
retries ahead of the change that supplies cloud hosts.
