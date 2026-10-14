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
- Let each local runtime open the declared host ports it needs to, so a runtime
  behind a VM boundary publishes them and a runtime that binds directly does
  not.
- Reconcile on start, including an already-running deployment, stop sidecars
  with the database, and clean up on destroy.
- Report running state, reconciliation outcome, endpoints, and unresolved
  operation failures.
- Report a short cause when a sidecar fails to start and point the user at
  `exasol sidecar status` for the full diagnosis.
- Report sidecars as unsupported on deployments whose backend does not run
  them.

## Capabilities

### New Capabilities

- `sidecar-networking`: Deployment-scoped DNS connectivity and published
  loopback endpoints.
- `sidecar-lifecycle`: Enablement, reconciliation, restart behavior, status,
  independent removal, and deployment cleanup.

## Impact

The change affects deployment lifecycle orchestration and locking, Podman
execution through the local execution environments, and platform-specific
publication. The deployment backend gains a sidecar capability that local
deployments answer, applying a desired set and reporting what it observes.

User and contributor documentation and the unreleased changelog gain the
sidecar workflow. Tests use the fixture catalog on the existing local lanes.
