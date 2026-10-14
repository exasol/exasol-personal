# Developing sidecars

See the [architecture overview](architecture.md) for ownership
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

## Running container commands

Every container command goes through the
[container client](../internal/localinstall/container_client.go), which takes
what the caller wants and decides how to ask for it. Callers describe a
container with a [`ContainerSpec`](../internal/localinstall/container_spec.go)
and name operations like `Create`, `Remove` or `Images`; no caller spells a
runtime flag.

Everything runtime-specific lives in
[the podman encoder](../internal/localinstall/container_podman.go): the
argument spelling, the pull and restart policy tables, and which commands
answer a diagnostic dump. A second runtime brings another encoder rather than
touching any call site.
## Running sidecar containers

Reuse the [sidecar driver](../internal/localinstall/sidecars.go) on that
client. It creates, inspects, compares, and cleans up containers carrying
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
