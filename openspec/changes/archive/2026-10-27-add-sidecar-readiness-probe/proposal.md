## Why

`exasol sidecar status` reports the container process. A container can be
running and fully reconciled while the service inside it is still starting,
misconfigured, or unable to reach the database. `exasol sidecar enable` returns
as soon as the container is created, so a client that connects right away can
fail, and a user cannot tell a service that is starting from one that is
broken.

## What Changes

- A saved or catalog container supports the Kubernetes `readinessProbe` field,
  with an `exec`, `httpGet`, or `tcpSocket` handler and the
  `initialDelaySeconds`, `periodSeconds`, `timeoutSeconds`, and
  `failureThreshold` timing fields.
- The launcher runs the probe when status or enable asks for it: `exec` inside
  the container, `httpGet` and `tcpSocket` against the port's published
  loopback address.
- Status reports `health` as `starting`, `serving`, or `failing` for a running
  sidecar that declares a readiness probe, separately from `state` and
  `reconciliation`, with the latest probe failure in `healthError`.
- `exasol sidecar enable` waits while a sidecar is starting, up to its probe's
  startup window, and reports the outcome with a call to action when it is not
  serving. `--no-wait` returns without waiting.
- The MCP catalog entry declares a readiness probe on its `/health` path.
- Editing the probe applies on the next status without recreating the
  container.

## Capabilities

### Modified Capabilities

- `sidecar-configuration`: containers accept a readiness probe with a
  validated handler and timing fields.
- `sidecar-lifecycle`: status reports health, and enable waits for a starting
  sidecar unless `--no-wait` is given.

## Impact

- The sidecar package gains the probe types, their validation, and their
  defaults.
- The container runtime gains a command execution operation and reports the
  container start time.
- Status in the deploy package evaluates the probe, and enable waits on it.
- `exasol sidecar enable` gains `--no-wait`, and text output gains health
  lines and calls to action.
- The embedded catalog, the user sidecar page, the CLI reference, and the
  changelog.
