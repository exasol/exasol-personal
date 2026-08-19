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

- **WHEN** a user runs `exasol install local` in an empty deployment directory on Windows AMD64 and approves any required host preparation
- **THEN** the launcher prepares Windows Podman, starts Nano through host Podman, waits up to a bounded timeout until the database accepts connections, and records the deployment as running

#### Scenario: Install local deployment times out

- **WHEN** a user runs `exasol install local` and the database does not become ready within the bounded timeout
- **THEN** the launcher fails the command rather than waiting indefinitely

#### Scenario: Reject unsupported local platform

- **WHEN** a user runs `exasol install local` on another operating system or architecture
- **THEN** the launcher fails before starting a runtime and lists the supported local platforms

### Requirement: Local lifecycle commands
The system SHALL support standard lifecycle commands for local deployments.

#### Scenario: Stop local deployment
- **WHEN** a local deployment is running and the user runs `exasol stop`
- **THEN** the launcher removes the disposable Nano container, stops a launcher-owned execution environment when applicable, preserves persistent data, and records the deployment as stopped

#### Scenario: Start local deployment
- **WHEN** a local deployment is stopped and the user runs `exasol start`
- **THEN** the launcher prepares and starts its execution environment, starts the shared Podman installation, waits up to a bounded timeout until the database accepts connections, refreshes connection artifacts, and records the deployment as running

#### Scenario: Start local deployment times out
- **WHEN** a local deployment is stopped, the user runs `exasol start`, and the database does not become ready within the bounded timeout
- **THEN** the launcher fails the command rather than waiting indefinitely

#### Scenario: Destroy local deployment
- **WHEN** a user runs `exasol destroy` for a local deployment
- **THEN** the launcher removes the disposable container, stops its launcher-owned execution environment if needed, deletes runtime data and connection artifacts, and records the deployment as initialized

### Requirement: Local configuration is platform-specific
The system SHALL expose and validate VM CPU, memory, and data-size configuration only for the macOS VM runtime, while Linux and Windows SHALL ignore persisted VM sizing and use their host Podman environments.

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

#### Scenario: Windows runtime memory has no endpoints
- **WHEN** Windows local status or health is requested after process restart and deployment information contains the database endpoint
- **THEN** the runtime recovers that endpoint and uses it for status and health reporting

#### Scenario: Windows database endpoint accepts connections
- **WHEN** the exact deployment container is running and the published database port accepts a connection
- **THEN** local health is healthy

### Requirement: Windows local shell commands fail explicitly
The system SHALL report that host and container shell access is unsupported for Windows host local deployments.

#### Scenario: Host shell requested on Windows
- **WHEN** a user runs `exasol shell host` for a Windows local deployment
- **THEN** the command fails with the runtime's explicit unsupported error

#### Scenario: Container shell requested on Windows
- **WHEN** a user runs `exasol shell container` for a Windows local deployment
- **THEN** the command fails with the runtime's explicit unsupported error
