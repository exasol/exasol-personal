# Developing sidecars

See the [architecture overview](architecture.md#sidecar-lifecycle) for
ownership boundaries.

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
configuration for exposure.

Catalog changes supply templates for new enablement. Existing deployments
retain their saved definitions, so test both a fresh enable and repeated
enable after editing the saved definition.

Use [catalog tests](../internal/sidecar/catalog_test.go) for loading,
architecture selection, and validation. Run `task tests-unit` and
`task tests-launcher`.

## Supplying templates from a command

Enablement resolves a name through a `Resolver`, so a command can offer
templates the embedded catalog does not carry. Build them with
`sidecar.NewCatalog`, which applies the same validation and defaults as a
catalog document, and pass the result to the enablement entry point that takes
runtime templates. Ordered `sidecar.Catalogs` resolve their first match, so
command-supplied entries take precedence while the embedded catalog still
answers every other name and architecture.

Saved definitions stay authoritative once a name is enabled, so a template
assembled for one command influences only the first enablement of that name.
Listing reports the merged entries, which keeps availability and enablement
consistent with whatever resolved the template.

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

Keep container networking in the adapter. Local hosts use the managed bridge;
cloud hosts set `DatabaseHost` for host networking and the `database` hosts
entry. Preserve deployment ownership labels and report applied runtime
endpoints so saved edits leave reported endpoints unchanged until
reconciliation applies them.

Cloud hosts install each container as a service of the node's deployment
service through `NewSystemdSidecarRuntime`. The node's service manager then
owns start, stop, and the container's restart policy, and resolved values
reach the container as container secrets created over private standard input.
Keep the unit and every launcher command free of credential values.

Opening an endpoint beyond the host that runs the container is the deployment
backend's job, through `OpenPorts` and `OpenedPorts` in
[port publication](../internal/deploy/sidecar_publication.go). The shared
service hands the backend every endpoint the deployment declares, each
annotated with the port the container runtime bound for it; a zero runtime
port means the endpoint is not currently served. A backend opens exactly that
set and withdraws what it no longer receives. Hosts whose container runtime
already binds the requested address open nothing and report no endpoints of
their own. A VM adapter opens host forwards to the guest binding. A cloud
backend admits the declared ports through the provider firewall, which follows
enablement rather than whether a container currently runs.

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
hashes for running sidecars, that the deployment serves exactly the declared
endpoints, and absence of containers and opened endpoints for cleanup.
Backends must report orphaned endpoints among their opened ports so cleanup
stays retryable. Keep runtime inspection read-only and report pending or
unknown reconciliation separately from running state.

Follow the [CLI output contract](best_practices.md#the-output-contract).
Return factual errors from adapters and queue recovery guidance in the command
layer through the CTA helper. Sidecar actions use deployment-file logging;
runtime progress stays on stderr and command results stay on stdout.
