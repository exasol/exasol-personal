## Why

A deployment that records which sidecars it wants still needs something to run
them. Giving local deployments a sidecar runtime turns saved definitions into
running services, with connectivity to the database and usable endpoints on the
user's own machine.

Related issue: [SPOT-32683](https://exasol.atlassian.net/browse/SPOT-32683).

## What Changes

- Run enabled sidecars as Podman containers on the deployment's local host,
  reconciling them against the saved definitions.
- Provide one named DNS-enabled bridge per deployment, where the database is
  reachable as `database` and each sidecar by its saved name.
- Open declared host ports through the deployment backend, so a runtime behind
  a VM boundary publishes them and a runtime that binds directly does not.
- Reconcile on start, including an already-running deployment, stop sidecars
  with the database, and clean up on destroy.
- Report per-host running state, reconciliation outcome, endpoints, and
  unresolved operation failures.

## Capabilities

### New Capabilities

- `sidecar-networking`: Deployment-scoped DNS connectivity and published
  loopback endpoints.
- `sidecar-lifecycle`: Enablement, reconciliation, restart behavior, status,
  independent removal, and deployment cleanup.

## Impact

The change affects deployment lifecycle orchestration and locking, Podman
execution through the local execution environments, and platform-specific
publication. Cloud deployments keep recording enablement until a later change
supplies their hosts.

User and contributor documentation and the unreleased changelog gain the
sidecar workflow. Tests use the fixture catalog on the existing local lanes.
