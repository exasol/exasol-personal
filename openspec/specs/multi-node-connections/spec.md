# multi-node-connections Specification

## Purpose
TBD - created by archiving change enable-multi-node-connections. Update Purpose after archive.
## Requirements
### Requirement: Connection selection follows saved deployment support

`exasol connect` SHALL use public node addresses for deployments that recorded support for connections on all nodes in their installation preset, and SHALL use the saved primary connection for unmarked deployments.

#### Scenario: Multiple eligible cloud nodes

- **WHEN** a user connects to a deployment that records support for connections on all nodes and exposes multiple public database endpoints with the same port
- **THEN** the launcher supplies all eligible node addresses to the database driver

#### Scenario: Unmarked deployment

- **WHEN** a user connects to a deployment whose saved installation preset omits multi-node support
- **THEN** the launcher uses its saved primary host and database port

#### Scenario: Local deployment

- **WHEN** a user connects to a local deployment
- **THEN** the launcher uses its saved local database endpoint

#### Scenario: Driver cannot represent the endpoints

- **WHEN** eligible database endpoints use different ports
- **THEN** the connection attempt fails with an error identifying the shared-port requirement

### Requirement: Eligible node connections verify the cluster certificate

`exasol connect` SHALL verify the stored cluster certificate fingerprint for each eligible endpoint it attempts under the default certificate validation policy.

#### Scenario: Transport failure of one endpoint

- **WHEN** an eligible endpoint rejects or stalls its transport handshake and another eligible endpoint is available with the stored cluster certificate
- **THEN** the launcher establishes a connection through the available endpoint within its overall connection deadline

#### Scenario: Certificate mismatch

- **WHEN** all available eligible endpoints present a certificate different from the stored fingerprint
- **THEN** the connection fails certificate verification

### Requirement: Older launchers retain access to new deployments

Within the existing deployment-version compatibility rules, deployment metadata created with multi-node support SHALL remain readable by the preceding launcher, which SHALL be able to use its primary-host connection path.

#### Scenario: Older launcher reads a new deployment

- **WHEN** the preceding launcher reads a version-compatible deployment initialized with multi-node support
- **THEN** it can report connection information and connect through the primary host using the existing metadata fields
