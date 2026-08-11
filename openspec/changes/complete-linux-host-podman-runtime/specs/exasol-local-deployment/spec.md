## MODIFIED Requirements

### Requirement: Exasol Local deployment preset

The system SHALL provide the standard local deployment option through a managed VM on macOS Apple Silicon and through host Podman on Linux AMD64 and ARM64.

#### Scenario: Install local deployment

- **WHEN** a user runs `exasol install local` in an empty deployment directory on macOS Apple Silicon
- **THEN** the launcher initializes the deployment directory, starts the Exasol Local VM, waits up to a bounded timeout until the database accepts connections, and records the deployment as running

#### Scenario: Install local deployment on Linux

- **WHEN** a user runs `exasol install local` in an empty deployment directory on Linux AMD64 or ARM64
- **THEN** the launcher initializes the deployment directory, starts Nano through host Podman, waits up to a bounded timeout until the database accepts connections, and records the deployment as running

#### Scenario: Install local deployment times out

- **WHEN** a user runs `exasol install local` and the database does not become ready within the bounded timeout
- **THEN** the launcher fails the command rather than waiting indefinitely

#### Scenario: Reject unsupported local platform

- **WHEN** a user runs `exasol install local` on another operating system or architecture
- **THEN** the launcher fails before starting a runtime and lists the supported local platforms

## ADDED Requirements

### Requirement: Local configuration is platform-specific
The system SHALL expose and validate VM CPU, memory, and data-size configuration only for the macOS VM runtime, while Linux SHALL ignore persisted VM sizing and use unrestricted host resources.

#### Scenario: Linux local configuration contains VM sizing
- **WHEN** a Linux deployment contains shared VM CPU, memory, or data-size values
- **THEN** local validation and startup ignore those values and do not advertise them as effective Linux settings

#### Scenario: macOS local configuration contains invalid VM sizing
- **WHEN** a macOS Apple Silicon deployment contains invalid VM sizing
- **THEN** validation fails as before

### Requirement: Linux local status and health reflect the published database endpoint
The system SHALL derive Linux runtime status from the exact Podman deployment container, recover missing in-memory endpoints from deployment information, and probe the published database port for health.

#### Scenario: Runtime memory has no endpoints
- **WHEN** Linux local status or health is requested after process restart and deployment information contains the database endpoint
- **THEN** the runtime recovers that endpoint and uses it for status and health reporting

#### Scenario: Published database endpoint accepts connections
- **WHEN** the exact deployment container is running and the published database port accepts a connection
- **THEN** local health is healthy

#### Scenario: Linux database is unreachable
- **WHEN** the published Linux database endpoint cannot be reached
- **THEN** diagnostics describe the Linux host-container failure without macOS host-to-VM guidance

### Requirement: Linux local shell commands fail explicitly
The system SHALL return platform-specific unsupported errors for host and container shell commands when the selected runtime cannot provide shell access.

#### Scenario: Host shell requested on Linux
- **WHEN** a user runs `exasol shell host` for a Linux local deployment
- **THEN** the command fails with an explicit Linux host-runtime unsupported message

#### Scenario: Container shell requested on Linux
- **WHEN** a user runs `exasol shell container` for a Linux local deployment
- **THEN** the command fails with an explicit Linux host-runtime unsupported message
