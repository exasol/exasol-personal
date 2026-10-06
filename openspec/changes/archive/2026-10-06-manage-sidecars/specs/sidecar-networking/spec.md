## Purpose

Provide stable connectivity between a deployment's database and sidecars,
with usable loopback endpoints for services published to the local host.

## ADDED Requirements

### Requirement: Deployment services have stable DNS names

Each local deployment SHALL provide a shared service network where the database
is reachable as `database` and each enabled sidecar is reachable by its
catalog-derived name. Names SHALL remain stable across container recreation
and SHALL resolve within the owning deployment's network.

#### Scenario: Database and sidecar communicate through service names

- **WHEN** a sidecar is enabled for a running deployment
- **THEN** the sidecar resolves `database` and reaches its internal SQL port
- **AND** execution in the database's network context resolves the sidecar
  name and reaches its listening service

#### Scenario: Separate deployments reuse service aliases

- **WHEN** two deployments enable the same sidecar name
- **THEN** each deployment's service aliases resolve to its own containers

### Requirement: Container port mappings produce loopback endpoints

For local TCP ports with a positive `hostPort`, the launcher SHALL publish
`containerPort` on the requested loopback `hostIP` and host port, defaulting
an omitted host IP to `127.0.0.1`. A declaration containing only
`containerPort` SHALL provide an internal service port. Status SHALL report
the effective published host IP and port. Non-loopback publication requests
SHALL receive an explicit local-support validation error.

#### Scenario: Published endpoint is reachable from the local host

- **WHEN** a sidecar with a loopback host mapping is enabled
- **THEN** the service is reachable at the IP and port reported by status
- **AND** the host listener is bound exclusively to the requested loopback IP

#### Scenario: Internal service port stays internal

- **WHEN** a sidecar declares a container port with host publication omitted
- **THEN** its service is reachable by deployment DNS and status reports an
  empty published endpoint list

#### Scenario: Broad publication request receives a validation error

- **WHEN** a saved sidecar requests a non-loopback host IP
- **THEN** validation identifies that mapping and the supported loopback scope

### Requirement: Existing deployments can join sidecar networking

New local database containers SHALL use the deployment bridge before sidecars are
enabled. An existing bridge-backed database SHALL gain service-name
connectivity while preserving its active SQL service. For a running database
in a non-bridge network mode, enable SHALL save the sidecar definition and
report it enabled with `restartRequired: true`. Text output SHALL include a
call to action to run `exasol stop` followed by `exasol start`. JSON output
SHALL expose the restart requirement as structured data.

#### Scenario: Enabling joins an existing running database

- **WHEN** the first sidecar is enabled on a running bridge-backed deployment
- **THEN** the sidecar can reach `database` and the existing database process
  continues serving SQL through its published endpoint

#### Scenario: Existing non-bridge deployment waits for explicit restart

- **WHEN** a sidecar is enabled on a running non-bridge deployment
- **THEN** its definition is saved and status reports enabled, stopped, and
  restart required while the existing database continues serving SQL
- **AND** text output provides the stop/start call to action and JSON output
  exposes `restartRequired: true`
- **AND** the next explicit stop/start activates the bridge and sidecar

### Requirement: Cloud sidecars share the node network

Cloud sidecars SHALL use host networking with `database` resolving to the
node's private database address. A positive `hostPort` SHALL equal
`containerPort`. The service's own listening configuration SHALL determine
its network exposure. Status SHALL report declared endpoints from the applied
container definition in the node's network context.

#### Scenario: Cloud container receives host networking and database name

- **WHEN** a cloud sidecar starts with a declared endpoint
- **THEN** its runtime uses host networking and resolves `database` to the node
- **AND** its reported endpoint matches the applied definition

#### Scenario: Cloud remapping receives a validation error

- **WHEN** a cloud sidecar declares different positive host and container ports
- **THEN** startup reports that host networking requires equal ports before
  changing runtime containers
