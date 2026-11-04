# sidecar-networking Specification

## Purpose
Provide stable connectivity between a deployment's database and sidecars,
with usable loopback endpoints for services published to the local host.
## Requirements
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
the effective published host IP and port. When a published port declares an
application protocol, status SHALL additionally report a complete URL built
from that scheme, the effective published host address, and any declared path.
Non-loopback publication requests SHALL receive an explicit local-support
validation error.

#### Scenario: Published endpoint is reachable from the local host

- **WHEN** a sidecar with a loopback host mapping is enabled
- **THEN** the service is reachable at the IP and port reported by status
- **AND** the host listener is bound exclusively to the requested loopback IP

#### Scenario: Published endpoint reports its complete URL

- **WHEN** a running sidecar publishes a port that declares an application
  protocol
- **THEN** text and JSON status report a URL carrying that scheme, the
  effective published host address, and the declared path
- **AND** the service answers at that URL

#### Scenario: Published endpoint without a declaration reports its address

- **WHEN** a running sidecar publishes a port that declares no application
  protocol
- **THEN** status reports that endpoint's host IP and port and no URL

#### Scenario: Internal service port stays internal

- **WHEN** a sidecar declares a container port with host publication omitted
- **THEN** its service is reachable by deployment DNS and status reports an
  empty published endpoint list
- **AND** an application protocol declared for that port leaves that list empty

#### Scenario: Broad publication request receives a validation error

- **WHEN** a saved sidecar requests a non-loopback host IP
- **THEN** validation identifies that mapping and the supported loopback scope

### Requirement: Existing deployments can join sidecar networking

New local database containers SHALL use the deployment bridge before
sidecars are enabled when the host supports it. An existing bridge-backed
database SHALL gain service-name connectivity while preserving its active SQL
service. For a running database in a non-bridge network mode, enable SHALL save
the sidecar definition and report it enabled with `restartRequired: true`. Text
output SHALL include a call to action to run `exasol stop` followed by
`exasol start`. JSON output SHALL expose the restart requirement as structured
data.

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

### Requirement: Hosts without service network support keep the database running

Start SHALL start the database without the deployment bridge when it cannot
be started on the bridge because the host's kernel lacks the required nftables
support. Enabling a sidecar on such a running deployment SHALL fail without
saving the sidecar and keep the database running. Starting such a deployment
with an enabled sidecar SHALL start the database and report each enabled
sidecar as failed, as for any other sidecar start failure, and status SHALL
report each enabled sidecar as failed. On Windows and inside WSL, these
failures SHALL state that sidecars on Podman require WSL 2.7 or later, and the
text output of enable and start SHALL include a call to action to run
`wsl --update`, then `exasol stop` and `exasol start`. On other hosts, the
failures SHALL name the kernel's missing nftables support, and the call to
action SHALL link to the troubleshooting documentation. A later start that succeeds on the bridge SHALL
make sidecars available again.

#### Scenario: A deployment without sidecars starts on WSL 2.6

- **WHEN** a local Podman deployment without sidecars is installed or started
  on WSL 2.6
- **THEN** the command succeeds and the database serves SQL

#### Scenario: Enable on WSL 2.6 reports the WSL requirement

- **WHEN** a sidecar is enabled on a running Podman deployment on WSL 2.6
- **THEN** the command fails with the WSL 2.7 requirement and a call to action
  to run `wsl --update`, `exasol stop` and `exasol start`, and the database
  keeps serving SQL
- **AND** the sidecar is not enabled

#### Scenario: Start with an enabled sidecar on WSL 2.6 reports the sidecar failed

- **WHEN** a Podman deployment with an enabled sidecar is started on WSL 2.6
- **THEN** the database serves SQL, the start fails reporting the sidecar with
  the WSL 2.7 requirement and the same call to action, and status reports the
  sidecar failed with the WSL 2.7 requirement

#### Scenario: Sidecars become available after the host is fixed

- **WHEN** WSL is updated to 2.7 or later, or Podman is switched to iptables,
  and the deployment is stopped and started
- **THEN** enabling a sidecar succeeds without a further restart

