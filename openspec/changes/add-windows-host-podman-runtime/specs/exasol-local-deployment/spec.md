## MODIFIED Requirements

### Requirement: Exasol Local deployment preset

The system SHALL provide the standard local deployment option through a VM-backed Podman installation on macOS Apple Silicon and host Podman on Linux AMD64, Linux ARM64, and Windows AMD64.

#### Scenario: Install local deployment

- **WHEN** a user runs `exasol install local` in an empty deployment directory on macOS Apple Silicon
- **THEN** the launcher initializes and starts the VM with application forwards, installs Nano through guest Podman, waits up to a bounded timeout until the database accepts connections, and records the deployment as running

#### Scenario: Install local deployment on Linux

- **WHEN** a user runs `exasol install local` in an empty deployment directory on Linux AMD64 or ARM64
- **THEN** the launcher initializes the deployment directory, starts Nano through host Podman, waits up to a bounded timeout until the database accepts connections, and records the deployment as running

#### Scenario: Install local deployment on Windows

- **WHEN** a user runs `exasol install local` in an empty deployment directory on Windows AMD64
- **THEN** the launcher prepares Podman and its default machine, initializes the deployment directory, starts Nano through host Podman, waits up to a bounded timeout until the database accepts connections, and records the deployment as running

#### Scenario: Install local deployment times out

- **WHEN** a user runs `exasol install local` and the database does not become ready within the bounded timeout
- **THEN** the launcher fails the command rather than waiting indefinitely

#### Scenario: Reject unsupported local platform

- **WHEN** a user runs `exasol install local` on another operating system or architecture
- **THEN** the launcher fails before starting a runtime and lists the supported local platforms

### Requirement: Local configuration is platform-specific
The system SHALL expose and validate VM CPU, memory, and data-size configuration only for the macOS VM runtime, while direct-host platforms SHALL ignore persisted VM sizing and use host-managed resources.

#### Scenario: Linux local configuration contains VM sizing
- **WHEN** a Linux deployment contains shared VM CPU, memory, or data-size values
- **THEN** local validation and startup ignore those values and do not advertise them as effective Linux settings

#### Scenario: Windows local configuration contains VM sizing
- **WHEN** a Windows deployment contains shared VM CPU, memory, or data-size values
- **THEN** local validation and startup ignore those values and do not advertise them as effective Windows settings

#### Scenario: macOS local configuration contains invalid VM sizing
- **WHEN** a macOS Apple Silicon deployment contains invalid VM sizing
- **THEN** validation fails as before

## ADDED Requirements

### Requirement: Windows local status and health reflect the published database endpoint
The system SHALL derive Windows runtime status from the exact Podman deployment container, recover missing in-memory endpoints from deployment information, and probe the published database port for health.

#### Scenario: Endpoint is recovered after process restart
- **WHEN** Windows local status or health is requested after process restart and deployment information contains the database endpoint
- **THEN** the runtime reports status and health against the recorded published database port

#### Scenario: Windows database is unreachable
- **WHEN** the published Windows database endpoint cannot be reached
- **THEN** diagnostics describe the container-to-machine-to-host forwarding path without macOS host-to-VM guidance

#### Scenario: Windows local runtime state is reported
- **WHEN** local diagnostics report a Windows deployment's runtime
- **THEN** they describe a Podman container rather than a launcher-owned VM

### Requirement: Windows local shell commands fail explicitly
The system SHALL reject host and container shell requests for Windows local deployments with an explicit unsupported error.

#### Scenario: Host shell requested on Windows
- **WHEN** a user runs `exasol shell host` for a Windows local deployment
- **THEN** the command fails with an explicit host-runtime unsupported message

#### Scenario: Container shell requested on Windows
- **WHEN** a user runs `exasol shell container` for a Windows local deployment
- **THEN** the command fails with an explicit host-runtime unsupported message

### Requirement: Local database endpoints are published on the IPv4 loopback
The system SHALL publish the local database port on the IPv4 loopback address rather than on all addresses.

#### Scenario: Database container is started
- **WHEN** the launcher starts a local deployment's database container
- **THEN** the configured database port is published on the IPv4 loopback address only

#### Scenario: Client resolves loopback to IPv6 on Windows
- **WHEN** a client on Windows resolves the loopback name while the Podman machine has no IPv6 route to the container
- **THEN** the published endpoint remains reachable because it is not exposed as an IPv6-only endpoint

### Requirement: Windows local deployments do not own the shared Podman machine
The system SHALL treat Podman's default machine as host-wide state that outlives an individual deployment.

#### Scenario: Windows local deployment is stopped
- **WHEN** a user runs `exasol stop` for a Windows local deployment
- **THEN** the deployment's container is removed and the Podman machine keeps running

#### Scenario: Windows local deployment is destroyed
- **WHEN** a user runs `exasol destroy` for a Windows local deployment
- **THEN** the deployment's container and local runtime files are removed and the Podman machine is neither stopped nor removed
