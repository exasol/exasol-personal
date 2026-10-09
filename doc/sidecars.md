# Developing sidecars

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
