## MODIFIED Requirements

### Requirement: Exasol Local deployment preset

The system SHALL provide the standard local deployment option through a VM-backed Podman installation on macOS Apple Silicon and host Podman on Linux AMD64 and ARM64.

#### Scenario: Install local deployment

- **WHEN** a user runs `exasol install local` in an empty deployment directory on macOS Apple Silicon
- **THEN** the launcher initializes and starts the VM with application forwards, installs Nano through guest Podman, waits up to a bounded timeout until the database accepts connections, and records the deployment as running

#### Scenario: Install local deployment on Linux

- **WHEN** a user runs `exasol install local` in an empty deployment directory on Linux AMD64 or ARM64
- **THEN** the launcher initializes the deployment directory, starts Nano through host Podman, waits up to a bounded timeout until the database accepts connections, and records the deployment as running

#### Scenario: Install local deployment times out

- **WHEN** a user runs `exasol install local` and the database does not become ready within the bounded timeout
- **THEN** the launcher fails the command rather than waiting indefinitely

#### Scenario: Reject unsupported local platform

- **WHEN** a user runs `exasol install local` on another operating system or architecture
- **THEN** the launcher fails before starting a runtime and lists the supported local platforms

### Requirement: Local deployment artifacts

The system SHALL write standard launcher artifacts for local deployments so existing information, connection, status, and shell commands can operate on the deployment directory.

#### Scenario: Connection artifacts are written after startup

- **WHEN** the local Podman installation starts successfully
- **THEN** the launcher writes `deployment.json`, `secrets.json`, and connection instructions with loopback application connection details

#### Scenario: Local credentials are available

- **WHEN** the launcher writes secrets for a local deployment
- **THEN** `secrets.json` contains database credentials for user `sys` with password `exasol`

#### Scenario: Forwarded ports are refreshed

- **WHEN** a macOS local deployment is started after being stopped
- **THEN** the launcher refreshes `deployment.json` with the current labeled database and UI forwards without recording an SSH endpoint

### Requirement: Local lifecycle commands
The system SHALL support standard lifecycle commands for local deployments.

#### Scenario: Stop local deployment
- **WHEN** a macOS local deployment is running and the user runs `exasol stop`
- **THEN** the launcher removes the disposable Nano container, stops the VM, preserves persistent data, and records the deployment as stopped

#### Scenario: Start local deployment
- **WHEN** a local deployment is stopped and the user runs `exasol start`
- **THEN** the launcher starts its execution environment, starts the shared Podman installation, waits up to a bounded timeout until the database accepts connections, refreshes connection artifacts, and records the deployment as running

#### Scenario: Start local deployment times out
- **WHEN** a local deployment is stopped, the user runs `exasol start`, and the database does not become ready within the bounded timeout
- **THEN** the launcher fails the command rather than waiting indefinitely

#### Scenario: Destroy local deployment
- **WHEN** a user runs `exasol destroy` for a local deployment
- **THEN** the launcher removes the disposable container, stops its execution environment if needed, deletes runtime data and connection artifacts, and records the deployment as initialized

### Requirement: Local shell access
The system SHALL provide shell access for local deployments through the selected local runtime.

#### Scenario: Host shell
- **WHEN** a macOS local deployment is running and the user runs `exasol shell host`
- **THEN** the runtime opens an interactive shell in the VM without exposing the underlying transport

#### Scenario: Container shell
- **WHEN** a macOS local deployment is running and the user runs `exasol shell container`
- **THEN** the runtime opens an interactive VM shell against the deployment's mounted Nano rootfs and container namespaces without exposing the underlying transport
