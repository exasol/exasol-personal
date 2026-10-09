## 1. Cloud runtime

- [x] 1.1 Install cloud containers as services of the node's deployment
  service, with the node's service manager owning start, stop, and the
  container's restart policy. Deliver resolved values as container secrets
  created over private standard input, keeping the unit and every launcher
  command free of credential values. Use host networking with the `database`
  hosts entry and require a declared `hostPort` to equal `containerPort`.
  Verify service binding, secret delivery, converged reuse, cleanup, host
  networking, and remapping validation with `task tests-unit`.

## 2. Cloud hosts and ingress

- [x] 2.1 Discover one sidecar host per cloud node with its identity,
  architecture, availability, database connection values, and container
  manager, applying definitions through the existing SSH transport with
  environment overrides sent through private standard input. Admit declared
  host ports through every infrastructure preset's ingress rules for the
  deployment's allowed address range, following enablement rather than running
  state and surviving reconfiguration. Verify node identity and connection,
  private input, SSH retry behavior, and the admitted-port configuration with
  `task tests-unit`.

## 3. Validation and documentation

- [x] 3.1 Validate cloud sidecars on every provisioned node with the pinned
  Caddy fixture catalog, covering environment delivery, the database alias,
  edited definitions, missing-container recovery, firewall admission across an
  explicit stop and start, the node's own service restart, and cleanup. Verify
  the cloud sidecar evidence with cloud credentials configured.
- [x] 3.2 Document cloud sidecar networking, firewall admission, and the
  node's deployment service in the user and contributor guides. Verify
  `task docs-check` and `task docs-build`.

## Commit and completion boundaries

Each numbered task is one independently verifiable commit. Keep updates to
existing behavior documentation with their implementation. Run the full
relevant checks for each commit. Keep all files in this active change
uncommitted and update checkboxes locally as work completes.

CI owns full cloud deployment validation using the pinned Caddy fixture
catalog. External workflow dispatch requires confirmation.

After all implementation tasks pass, sync the delta specs, archive this change,
and make the pure OpenSpec result the final branch commit. Preserve the user's
one-line Conventional Commit format.

## Scenario-to-test ownership

Cloud scenarios below are ADDED; stop, restart, and status scenarios are
MODIFIED. Each has exactly one owning test, identified for review with the
`openspec` marker on live tests.

### sidecar-lifecycle

- Cloud enablement precedes provisioning:
  `test_sidecar_cloud_enable_before_provisioning` (launcher).
- Hosts reconcile independently after a failure:
  `TestSidecarEveryHostReconcilesIndependently` (deployment unit).
- Disable retries a failed host independently:
  `TestSidecarMultiHostDisableRetriesFailedHost` (deployment unit).
- Cloud node follows its own database service:
  `test_sidecar_cloud_service_restart` (live, cloud).

### sidecar-networking

- Cloud container receives host networking and database name:
  `TestSidecarCloudHostNetworking` (runtime contract unit).
- Cloud remapping receives a validation error:
  `TestSidecarCloudRejectsRemappingBeforeRuntimeChanges` (runtime contract unit).
- Declared cloud endpoint is admitted for its deployment:
  `test_sidecar_cloud_declared_endpoint_admitted` (live, cloud).

### Fixture and execution details

Cloud evidence lives in `tests/live/test_sidecar_cloud.py` and uses the
reusable live deployment. The supplementary
`test_sidecar_cloud_nodes_reconcile` exercises host networking and the shared
definition on every real node, including environment delivery, edited
definitions, missing-container recovery, and disable cleanup. The admission
test binds the service to every address and probes each node's public address.
The service test drives the node's own database service. Unit tests keep
deployment and runtime dependencies isolated, including the unit and secret
contracts of the cloud transport and the ingress value the cloud backend
maintains.
