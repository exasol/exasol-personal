# Developing sidecars

See the [architecture overview](architecture.md#sidecar-lifecycle) for ownership
boundaries and the [user guide](../user-docs/content/sidecars.md) for supported
Container fields, connection references, and networking behavior.

## Adding a catalog entry

Edit the embedded [catalog](../assets/resources/sidecar-catalog.yaml). Add a
self-contained entry under `sidecars.<name>` with `description`,
`architectures`, and `container`. Use a DNS label for the key; `database` is
reserved. Omit `container.name` to inherit the key, or set it to the same name.

Declare the image's supported Linux target architectures (`amd64`, `arm64`),
including when the launcher runs on a different operating system. Prefer a
pinned multi-architecture image digest and verify each declared architecture.
Use the supported Container subset and let the loader apply defaults.

Use `env[].valueFrom.secretKeyRef` for runtime database connection values.
Choose environment names expected by the application. Include only the
connection keys it needs. Configure the application to retry database
connections because sidecar startup can precede SQL readiness.

For entries intended for both local and cloud deployments, use equal
`hostPort` and `containerPort` values. Configure the application's listening
address explicitly: cloud host networking relies on the application's bind
configuration for exposure. Review the user guide's networking contract when
choosing that address.

Catalog changes supply templates for new enablement. Existing deployments
retain their saved definitions, so test both a fresh enable and repeated
enable after editing the saved definition.

Use [catalog tests](../internal/sidecar/catalog_test.go) for loading,
architecture selection, and validation. Run `task tests-unit` and
`task tests-integration`. For runtime checks, follow the
[sidecar fixture build instructions](../tests/README.md#sidecar-fixture-builds).
The [fixture catalog](../tests/fixtures/sidecars/catalog.yaml) demonstrates
connection references using disposable test credentials.

## Adding a host adapter

Implement the backend's `SidecarHosts` contract in
[host discovery](../internal/deploy/sidecar_hosts.go). Return stable host names,
the container target architecture, availability, the internal database port,
and a `SidecarProvider`. An initialized deployment may have no hosts yet.
Discover the current node set after provisioning and refresh it on start.

Reuse the [Podman manager](../internal/localinstall/sidecars.go) through a
`CommandRunner` that preserves arguments, streams, cancellation, and
command failures. `Run` accepts optional child-process environment
overrides. Pass every resolved container environment value through this map
and use `--env NAME` in Podman arguments. Direct execution sets the child
environment; runner and SSH adapters transport overrides through stdin while
preserving the command's input. Keep credentials out of process arguments and
launcher command logs.
Transport retries must distinguish connection failure from a command that
may already have executed.

Keep networking and publication in the adapter. Local hosts use the managed
bridge; cloud hosts set `DatabaseHost` for host networking and the `database`
hosts entry. VM adapters also reconcile host forwards when containers are
created or removed. Preserve deployment ownership labels and report applied
runtime endpoints so saved edits leave reported endpoints unchanged until
reconciliation applies them.

Wire the shared, sidecar-specific
[lifecycle hooks](../internal/deploy/sidecar_lifecycle.go):

- `HostsReady` after command execution becomes available, including an
  already-running start. Invoke it before the startup SQL-readiness wait.
- `BeforeStop` while hosts remain reachable. Continue database shutdown when
  sidecar cleanup fails and report both outcomes separately.
- `AfterDestroy` after successful destruction, clearing observed failures
  while retaining saved definitions for redeployment.

Use the shared deployment operations and locking for enablement, saved state,
reconciliation, and per-host errors. Adapter tests should cover private
transport, network arguments, endpoint reporting, cleanup, and independent
recovery when one host fails. Follow the
[host contract tests](../internal/deploy/sidecar_hosts_test.go), then verify
reachability and stop/start behavior on the target runtime using disposable
deployments.

Reconciliation compares the defaulted saved definition's hash with the live
container's applied-definition label and running state. Runtime-resolved
connection values take effect on recreation. Treat `sidecars-state.json` as
unresolved diagnostics and cleanup hints. Status takes the exclusive lock to
clear failures after verifying the desired outcome. Verify applied-definition
hashes and published mappings for running sidecars, and absence of containers
and forwards for cleanup. Host adapters must expose incomplete publication and
orphaned forwards in their observations. Keep runtime inspection read-only and
report pending or unknown reconciliation separately from running state.

Follow the [CLI output contract](best_practices.md#the-output-contract).
Return factual errors from adapters and queue recovery guidance in the command
layer through the CTA helper. Sidecar actions use deployment-file logging;
runtime progress stays on stderr and command results stay on stdout.
