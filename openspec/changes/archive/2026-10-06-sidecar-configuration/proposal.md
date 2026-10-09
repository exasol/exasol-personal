## Why

Services that run beside an Exasol Personal database need configuration a user
can inspect and edit before any of them is started. Establishing the catalog,
the deployment-owned definition file, and the commands over it gives that
configuration a stable shape, and gives the runtime work that follows a single
authority for what a deployment wants to run.

Related issue: [SPOT-32683](https://exasol.atlassian.net/browse/SPOT-32683).

## What Changes

- Add `exasol sidecar list`, `enable <name>`, `status <name>`, and
  `disable <name>`, with `--json`.
- Embed a catalog of self-contained sidecar templates using a supported subset
  of the Kubernetes Container schema.
- Save enabled container definitions in deployment-owned `sidecars.yaml`,
  accept user edits, and use saved definitions as the authority for later
  operations.
- Resolve a selection through an ordered resolver, so entries assembled at
  runtime share the catalog's validation and defaults.
- Resolve database connection references and expand command and argument
  strings with Kubernetes semantics.

## Capabilities

### New Capabilities

- `sidecar-configuration`: Catalog lookup, saved Container definitions,
  validation, and database connection references.

## Impact

The change affects CLI registration, embedded resources, and deployment
configuration and locking. Status reports an empty host array until a later
change supplies sidecar hosts.

The generated CLI reference and the unreleased changelog gain the sidecar
commands.

Container definitions retain standard field names for future Kubernetes
integration.
