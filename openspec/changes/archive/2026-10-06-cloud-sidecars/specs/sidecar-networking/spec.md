## Purpose

Provide stable connectivity between a deployment's database and sidecars,
with usable loopback endpoints for services published to the local host.

## ADDED Requirements

### Requirement: Cloud sidecars share the node network

Cloud sidecars SHALL use host networking with `database` resolving to the
node's private database address. A positive `hostPort` SHALL equal
`containerPort`. The service's own listening configuration SHALL determine
its network exposure. Status SHALL report declared endpoints from the applied
container definition in the node's network context.

A positive `hostPort` SHALL be admitted by the deployment's provider firewall
for its allowed address range on every node, alongside the deployment's
database ports. Admitted ports SHALL follow enablement, remaining admitted
across deployment stop and start. Disabling a sidecar SHALL withdraw the ports
that only it declared.

#### Scenario: Cloud container receives host networking and database name

- **WHEN** a cloud sidecar starts with a declared endpoint
- **THEN** its runtime uses host networking and resolves `database` to the node
- **AND** its reported endpoint matches the applied definition

#### Scenario: Cloud remapping receives a validation error

- **WHEN** a cloud sidecar declares different positive host and container ports
- **THEN** startup reports that host networking requires equal ports before
  changing runtime containers

#### Scenario: Declared cloud endpoint is admitted for its deployment

- **WHEN** a sidecar declaring a host port is enabled on a cloud deployment
- **THEN** a service listening on that port is reachable from the deployment's
  allowed address range on every node, and remains reachable after an explicit
  stop and start
- **AND** disabling that sidecar leaves the port unreachable while the database
  continues serving SQL
