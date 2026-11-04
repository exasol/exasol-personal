## MODIFIED Requirements

### Requirement: Local deployments SHALL persist deterministic service ports

When a local deployment is initialized or reconfigured with an unset or automatic port mapping, the launcher SHALL select and persist a concrete host port for every exposed service. For each service, automatic allocation SHALL try its default port first, scan upward through `65535`, wrap to the lowest unprivileged port `1024`, and continue through the port immediately before the default. A selected port SHALL be available on every IPv4 and IPv6 loopback address resolved for `localhost` at selection time and SHALL differ from every host port saved for the deployment's sidecars. An explicit service port that is saved for one of the deployment's sidecars SHALL be rejected with an error naming the service, the port, and the sidecar.

#### Scenario: Default database port is available

- **WHEN** a user initializes a local deployment with the port mapping omitted
- **AND** database port `8563` is usable on every resolved localhost loopback address
- **THEN** the launcher persists `db:8563` as the deployment's concrete port mapping

#### Scenario: Allocation advances from the service default

- **WHEN** a user initializes a local deployment with the port mapping omitted
- **AND** the database default port and one or more immediately following ports are unavailable on at least one resolved localhost loopback address
- **THEN** the launcher persists the first following port that is usable on every resolved localhost loopback address

#### Scenario: Allocation wraps through the unprivileged range

- **WHEN** every port from a service's default through `65535` is unavailable
- **AND** a port is available between `1024` and the port immediately before the default
- **THEN** automatic allocation wraps to `1024` and persists the first usable port in that range

#### Scenario: No automatic port is available

- **WHEN** no port in the inclusive range `1024` through `65535` is usable for an exposed service
- **THEN** initialization fails and leaves the deployment uninitialized
- **AND** the error identifies the service for which allocation failed

#### Scenario: Explicit port mapping is preserved

- **WHEN** a user initializes or configures a local deployment with an explicit valid service-port mapping
- **THEN** the launcher persists that mapping unchanged

#### Scenario: Automatic port mapping is requested explicitly

- **WHEN** a user initializes a local deployment or reconfigures a stopped local deployment with `--ports auto`
- **THEN** the launcher selects and persists a concrete service-port mapping using the default-first wrapping order
- **AND** `exasol config get ports` reports the concrete mapping

#### Scenario: Stopped deployment port mapping is reset

- **WHEN** a user runs `exasol config reset ports` for a stopped local deployment
- **THEN** the launcher selects and persists a concrete service-port mapping using the default-first wrapping order
- **AND** the command reports the concrete mapping and guidance to run `exasol start`

#### Scenario: Automatic reconfiguration exhausts the port range

- **WHEN** a user requests automatic ports for a stopped local deployment
- **AND** no port in the inclusive range `1024` through `65535` is usable for an exposed service
- **THEN** the command fails and preserves the deployment's current port mapping
- **AND** the deployment remains stopped
- **AND** the error identifies the service for which allocation failed

#### Scenario: Multiple services receive distinct ports

- **WHEN** a local deployment exposes more than one service
- **THEN** automatic allocation starts from each service's own default port
- **AND** the launcher assigns distinct host ports to the services

#### Scenario: Automatic allocation skips sidecar ports

- **WHEN** a user requests automatic ports for a stopped local deployment
- **AND** the database default port is saved as a host port of one of its sidecars
- **THEN** the launcher persists a database port that differs from every saved sidecar host port

#### Scenario: Explicit port saved for a sidecar is rejected

- **WHEN** a user configures a local deployment with an explicit database port that is saved as a host port of one of its sidecars
- **THEN** the command fails and preserves the deployment's current port mapping
- **AND** the error names the service, the port, and the sidecar
