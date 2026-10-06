## Why

Additional services need consistent configuration, connectivity, and lifecycle
management alongside an Exasol Personal deployment. A shared sidecar contract
lets users operate catalog-supported services through the launcher.

Related issue: [SPOT-32683](https://exasol.atlassian.net/browse/SPOT-32683).

## What Changes

- Add `exasol sidecar list`, `enable <name>`, `status <name>`, and
  `disable <name>` for local and cloud deployments.
- Embed a catalog of self-contained sidecar templates using a supported subset
  of the Kubernetes Container schema.
- Save enabled container definitions in deployment-owned `sidecars.yaml`.
  Accept user edits and use saved definitions as the runtime authority.
- Keep advisory reconciliation diagnostics in `sidecars-state.json` and
  refresh unresolved failures when live inspection verifies desired state.
  Report per-host reconciliation separately from container running state.
- Resolve database connection environment references at container creation,
  with a password-injection opt-out.
- Provide one sidecar host locally and one per cloud node. Use a deployment
  bridge locally and host networking in cloud, with a stable `database` name.
- Reconcile enabled sidecars on deployment start, including an already-running
  deployment, and stop them with the database. Report state and failures per
  host, with one deployment-wide saved definition.
- Support `exasol shell host` on Linux through the user's local shell and
  execute explicit host commands directly for service inspection.
- Validate the common contract with a pinned off-the-shelf Caddy test image
  across supported local runtimes and cloud nodes.

## Capabilities

### New Capabilities

- `sidecar-configuration`: Catalog lookup, saved Container definitions,
  validation, and runtime connection references.
- `sidecar-networking`: Deployment-scoped DNS connectivity and published ports.
- `sidecar-lifecycle`: Enablement, reconciliation, restart behavior, status,
  independent removal, and deployment cleanup.

### Modified Capabilities

- `exasol-local-deployment`: Linux local host-shell and command execution.

## Impact

The change affects CLI registration, embedded resources, deployment
configuration and locking, shared lifecycle orchestration, Podman execution
through local and SSH transports, and platform-specific networking.

User documentation, generated CLI reference, and the unreleased changelog gain
the sidecar workflow. Tests use a fixture catalog and the existing Linux,
macOS, Windows, and cloud deployment lanes.

Container definitions retain standard field names for future Kubernetes
integration.
