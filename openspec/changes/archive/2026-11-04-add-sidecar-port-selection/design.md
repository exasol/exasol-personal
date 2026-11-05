## Context

The database selects its host port with the local port allocator: it tries the
service default, scans upward through `65535`, wraps to `1024`, and probes
every loopback address resolved for `localhost`. The result is saved in the
infrastructure manifest as a `<service>:<port>` mapping, and start binds that
port only. Sidecars save their ports in `sidecars.yaml` as Kubernetes `ports`
entries and publish `hostPort` as saved, with no selection step. On macOS the
VM runtime opens a host-side listener for each published sidecar port, so the
host is where a port is held in every runtime.

## Goals / Non-Goals

**Goals:**

- One port selection algorithm for the database and sidecars.
- `sidecars.yaml` stays the only place that stores sidecar host ports.

**Non-Goals:**

- Publishing an internal sidecar port through `--ports`. A port without a
  positive `hostPort` keeps its internal-only meaning.
- Changing `hostIP` from the CLI.
- Probing explicit host ports before saving them. The actual bind decides, as
  it does for the database.

## Decisions

### Selection reuses the local port allocator

The allocator gains a starting port and a reserved set as inputs, so callers
describe a sidecar port as a local service whose default is its starting port.
The database caller passes the saved sidecar host ports as reserved, and the
sidecar caller passes the saved database port and the other sidecars' host
ports. Selected ports for several ports of one sidecar are added to the
reserved set as they are chosen, which keeps them distinct.

### A sidecar's own running ports count as available

A running sidecar holds its own host ports, so reset would otherwise always
move it off its current port. When the sidecar's container
runs and publishes a saved host port, selection treats that port as available
without probing it. Reconciliation then reuses the container when the port is
unchanged.

### Explicit entries merge into the saved definition

`--ports` updates only the named ports, so a sidecar with several published
ports can move one without restating the others. Every entry names its port,
and automatic selection is reached through `config reset`.

### The starting port for automatic selection

Automatic selection at enable and reset starts at the catalog template's
`hostPort` for that port name. When the catalog no longer offers the sidecar
or the port name, it starts at the saved `hostPort`, so a user-added sidecar
keeps its port while it stays free.

### Config commands save first, then reconcile

`sidecar config set` and `reset` write `sidecars.yaml`, then reconcile the
sidecar when it is enabled and the deployment runs, using the same path as
enable. They report the saved values in the `<port-name>:<host-port>` form of
`exasol config`, and they return without waiting for readiness. `status`
reports health. `enable --ports` keeps enable's waiting behavior.

### Bind conflicts reuse the database classification

Sidecar container creation and VM port forwarding failures pass through the
same bind-failure classification the database uses. A recognized conflict
becomes a sidecar port error carrying the sidecar, port name, and host port,
and the CLI adds the recovery commands for it. Other failures keep the
existing startup failure reporting.

### Published ports require a name

The port name is the `--ports` key. The number is ambiguous because a
container port and a host port share one field. Validation therefore requires a
`name` on every port with a positive `hostPort`. Sidecars are unreleased, so
saved definitions need no migration.

## Risks / Trade-offs

- [A selected port can be taken between selection and bind] → The bind
  conflict reports the recovery commands, and `config reset` selects again.
- [The database reads `sidecars.yaml` during port selection] → An unreadable
  `sidecars.yaml` fails database reconfiguration with the sidecar validation
  error, which names the file's offending field.
