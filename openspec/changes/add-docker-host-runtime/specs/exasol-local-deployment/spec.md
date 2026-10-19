## MODIFIED Requirements

### Requirement: Exasol Local deployment preset

The system SHALL provide the standard local deployment option through a VM-backed Podman installation on macOS Apple Silicon, and through host Docker or host Podman on Linux AMD64, Linux ARM64, and Windows AMD64.

#### Scenario: Install local deployment

- **WHEN** a user runs `exasol install local` in an empty deployment directory on macOS Apple Silicon
- **THEN** the launcher initializes and starts the VM with application forwards, installs Nano through guest Podman, waits up to a bounded timeout until the database accepts connections, and records the deployment as running

#### Scenario: Install local deployment on Linux

- **WHEN** a user runs `exasol install local` in an empty deployment directory on Linux AMD64 or ARM64
- **THEN** the launcher initializes the deployment directory, starts Nano through host Docker when it is installed or host Podman otherwise, waits up to a bounded timeout until the database accepts connections, and records the deployment as running

#### Scenario: Install local deployment on Windows

- **WHEN** a user runs `exasol install local` in an empty deployment directory on Windows AMD64
- **THEN** the launcher initializes the deployment directory, starts Nano through host Docker when it is installed or otherwise prepares Podman and its default machine and starts Nano through host Podman, waits up to a bounded timeout until the database accepts connections, and records the deployment as running

#### Scenario: Install local deployment times out

- **WHEN** a user runs `exasol install local` and the database does not become ready within the bounded timeout
- **THEN** the launcher fails the command rather than waiting indefinitely

#### Scenario: Reject unsupported local platform

- **WHEN** a user runs `exasol install local` on another operating system or architecture
- **THEN** the launcher fails before starting a runtime and lists the supported local platforms

### Requirement: Local status reports a stopped runtime environment
The system SHALL report a local deployment as stopped when the environment hosting its
container is not running, and SHALL resolve that status without starting the environment.

#### Scenario: Windows Podman machine is not running

- **WHEN** a local deployment's recorded state is running, the Podman machine is not running,
  and the user runs `exasol status`
- **THEN** the command reports status `stopped` with guidance to run `start` to restart or
  `destroy` to delete resources
- **AND** the Podman machine remains stopped

#### Scenario: Docker daemon is not running

- **WHEN** a Docker deployment's recorded state is running, the Docker daemon does not answer,
  and the user runs `exasol status`
- **THEN** the command reports status `stopped` with guidance to run `start` to restart or
  `destroy` to delete resources
- **AND** the launcher log records why the daemon did not answer

#### Scenario: Stopped environment is reported within the status timeout

- **WHEN** `exasol status` resolves a local deployment whose environment is not running under
  the default timeout
- **THEN** the command completes within that timeout

#### Scenario: Stopped environment is reported in machine-readable output

- **WHEN** a user runs `exasol status --json` for a local deployment whose environment is not
  running
- **THEN** the output reports the same status value and message as the text output
