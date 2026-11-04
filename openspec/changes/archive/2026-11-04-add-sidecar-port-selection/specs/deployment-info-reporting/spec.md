## ADDED Requirements

### Requirement: Saved connection instructions follow sidecar changes

When `exasol sidecar enable`, `exasol sidecar disable`,
`exasol sidecar config set`, or `exasol sidecar config reset` changes a saved
sidecar of a running deployment, the launcher SHALL rewrite the deployment's
saved connection instructions so that their MCP server section matches the
saved sidecar definitions.

#### Scenario: Saved instructions follow a sidecar port change

- **WHEN** a user changes the MCP server's host port with
  `exasol sidecar config set` on a running deployment
- **THEN** the saved connection instructions show the MCP endpoint on the new
  port

### Requirement: Start prints connection instructions when only sidecars fail

When `exasol start` leaves the database running and one or more sidecars fail,
the command SHALL print the connection instructions, report each sidecar
failure, and exit with a nonzero status.

#### Scenario: Sidecar failure on start keeps the connection instructions

- **WHEN** `exasol start` starts the database and a sidecar fails to start
- **THEN** the output includes the connection instructions and the sidecar
  failure, and the command exits with a nonzero status
