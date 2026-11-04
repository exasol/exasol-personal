## ADDED Requirements

### Requirement: Host port changes apply immediately

When `exasol sidecar enable <name> --ports`, `exasol sidecar config set`, or
`exasol sidecar config reset` changes the saved host ports of an enabled
sidecar while the deployment runs, the command SHALL reconcile that sidecar
before it returns. For a disabled sidecar or a deployment that is not running,
the command SHALL save the host ports and the next start SHALL apply them.
For an enabled sidecar of a deployment that is not running, `config set` and
`config reset` SHALL tell the user to run `exasol start`, or `exasol deploy`
before the first deployment, to apply the change.

#### Scenario: Running sidecar moves to the selected port

- **WHEN** a user changes a running sidecar's host port with
  `exasol sidecar config set <name> --ports <port-name>:<port>`
- **THEN** after the command returns, status reports the sidecar's endpoint on
  the new port and the service answers there

#### Scenario: Stopped deployment applies the port on start

- **WHEN** a user changes a sidecar's host port while the deployment is stopped
- **THEN** the deployment stays stopped and the next start publishes the
  sidecar on the new port

#### Scenario: Port change on a stopped deployment names the apply command

- **WHEN** a user changes an enabled sidecar's host port with
  `exasol sidecar config set` while the deployment is stopped
- **THEN** the command tells the user to run `exasol start` to apply the
  change

### Requirement: Host port conflicts offer port recovery commands

When a sidecar cannot publish a saved host port because the port is already
held, the operation SHALL fail, keep the saved host ports, and identify the
sidecar, port name, and host port. Text output SHALL include
`exasol sidecar config set <name> --ports <port-name>:<available-port>` and
`exasol sidecar config reset <name> ports` as recovery commands.

#### Scenario: Explicit port is held by another process

- **WHEN** a user sets a running sidecar's host port to a port that another
  process holds
- **THEN** the command fails and `sidecars.yaml` keeps that host port
- **AND** the error names the sidecar, port name, and host port, and shows both
  recovery commands
