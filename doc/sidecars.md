# Developing sidecars

See the [architecture overview](architecture.md#sidecar-lifecycle) for ownership
boundaries and the [user guide](../user-docs/content/sidecars.md) for supported
Container fields, connection references, and networking behavior.

## Adding a catalog entry

Edit the embedded [catalog](../assets/resources/sidecar-catalog.yaml). Add a
self-contained entry under `sidecars.<name>` with `description`,
`architectures`, and `container`. Use a DNS label for the key; `database` is
reserved. Omit `container.name` to inherit the key, or set it to the same name.

A deployment's `sidecars.yaml` uses the same `sidecars.<name>` shape, holding a
`container` and optional `endpoints` under each key and applying the same
naming rule, so enablement stores a catalog entry's definition under the name
it was resolved by. Validation names a field through its key, for example
`sidecars.mcp.container.image`.

Declare the image's supported Linux target architectures (`amd64`, `arm64`),
including when the launcher runs on a different operating system. Prefer a
pinned multi-architecture image digest and verify each declared architecture.
Use the supported Container subset and let the loader apply defaults.

Declare an `endpoints` section beside `container` when a published port speaks
an application protocol, naming the port it applies to, so status reports a
complete URL. Enablement saves the declarations with the definition.

Use `env[].valueFrom.secretKeyRef` for runtime database connection values.
Choose environment names expected by the application. Include only the
connection keys it needs. Configure the application to retry database
connections because sidecar startup can precede SQL readiness.

Catalog changes supply templates for new enablement. Existing deployments
retain their saved definitions, so test both a fresh enable and repeated
enable after editing the saved definition.

Use [catalog tests](../internal/sidecar/catalog_test.go) for loading,
architecture selection, and validation. Run `task tests-unit` and
`task tests-launcher`. For runtime checks, see the
[sidecar fixture catalog](../tests/README.md#sidecar-fixture-catalog). The
[fixture definition](../tests/fixtures/sidecars/catalog.yaml) demonstrates
connection references using disposable test credentials.

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

## Running container commands

Every container command goes through the
[`ContainerRuntime` interface](../internal/localinstall/container.go). Callers
describe a container with a
[`ContainerSpec`](../internal/localinstall/container_spec.go) and name
operations like `Create`, `Remove` or `Images`; no caller spells a runtime
flag.

Everything runtime-specific lives in
[the podman backend](../internal/localinstall/container_podman.go): the
argument spelling, the pull and restart policy translations, the response
schemas it parses, and which commands answer a diagnostic dump. A second
runtime implements the interface rather than touching any call site.

## Running sidecar containers

Reuse the [sidecar driver](../internal/localinstall/sidecars.go) on that
runtime. It creates, inspects, compares, and cleans up containers carrying
deployment ownership labels, and reports applied runtime endpoints so saved
edits leave reported endpoints unchanged until reconciliation applies them.

Resolved connection values reach a container through an
[environment file](../internal/localinstall/sidecar_env.go) readable only by
its owner and removed once the container is created, so credentials stay out of
process arguments and launcher command logs. The file lives under the
deployment's shared directory, which a runtime behind a virtual machine
boundary reaches at its own mapped path. Values the file format cannot carry
are rejected naming the variable.

Keep container networking in the runtime. Local hosts use one managed bridge
per deployment, where the database answers to `database` and each sidecar to
its saved name.

Opening an endpoint beyond the host that runs the container belongs to the
runtime, through `OpenPorts` and `OpenedPorts` in
[the runtime contract](../internal/localruntime/runtime.go). A runtime opens
exactly the set it is handed and withdraws what it no longer receives. A
runtime whose containers bind on the user's machine opens nothing. A VM runtime
opens host forwards to the guest binding.

## Serving sidecars from a deployment backend

Serve sidecars from [deployment sidecars](../internal/deploy/sidecars.go),
which pairs a deployment with the runtime serving it. Sidecars are local only,
so the entry points resolve the deployment's backend kind and report any other
kind as unsupported rather than reaching through a shared interface.

`ApplySidecars` receives the whole desired set and makes the deployment serve
exactly it: create what is missing, replace what changed, remove what is no
longer desired, and publish the endpoints that set declares. Report each
sidecar's own failure in the result so status can attribute it, and reserve the
returned error for failures that affect the deployment as a whole. A backend
that cannot reach its host yet reports no application, which leaves recorded
diagnostics alone instead of clearing them against a runtime nobody saw.

`InspectSidecars` reports facts rather than verdicts: per sidecar, whether it
runs, its exit code, whether it was created from the definition that was
applied, and the endpoints a user can reach. Two of those need the backend's
own knowledge. Matching reads the salted definition label, and endpoints
resolve whatever the runtime published on top of what the container bound. The
comparison against saved definitions stays in the deployment layer, so status
has one definition of `complete`, `pending`, and `unknown`.

Deployment lifecycle integration lives in
[sidecar lifecycle](../internal/deploy/sidecar_lifecycle.go):

- Converge after command execution becomes available, including an
  already-running start. Do it before the startup SQL-readiness wait.
- Converge with an empty desired set while the host remains reachable, before
  stopping. Continue database shutdown when sidecar cleanup fails and report
  both outcomes separately.
- Remove recorded state after successful destruction, retaining saved
  definitions for redeployment.

Use the shared deployment operations and locking for enablement, saved state,
reconciliation, and per-sidecar errors. Backend tests should cover endpoint
resolution, cleanup, and the unsupported path. Follow the
[backend contract tests](../internal/deploy/sidecar_operations_test.go), then
verify reachability and stop/start behavior on the target runtime using
disposable deployments.

Reconciliation compares the defaulted saved definition's hash with the live
container's applied-definition label and running state. Runtime-resolved
connection values take effect on recreation. Treat `sidecars-state.json` as
unresolved diagnostics and cleanup hints. Status takes the exclusive lock to
clear failures after verifying the desired outcome. Verify applied-definition
hashes for running sidecars, that the deployment serves exactly the declared
endpoints, and absence of containers and opened endpoints for cleanup.
Runtimes must report orphaned endpoints among their opened ports so cleanup
stays retryable. Keep runtime inspection read-only and report pending or
unknown reconciliation separately from running state.

Follow the [CLI output contract](best_practices.md#the-output-contract).
Return factual errors from backends and queue recovery guidance in the command
layer through the CTA helper. A failed start carries its short cause and its
full diagnostic separately, so the command prints the cause and status reports
the diagnosis. Sidecar actions use deployment-file logging; runtime progress
stays on stderr and command results stay on stdout.
