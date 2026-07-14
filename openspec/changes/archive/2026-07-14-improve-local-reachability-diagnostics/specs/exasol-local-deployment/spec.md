## MODIFIED Requirements

### Requirement: Exasol Local deployment preset

The system SHALL provide a local deployment option for macOS Apple Silicon that manages the Exasol Local VM through the standard launcher workflow.

#### Scenario: Install local deployment

- **WHEN** a user runs `exasol install local` in an empty deployment directory on supported macOS Apple Silicon
- **THEN** the launcher initializes the deployment directory, starts the Exasol Local VM, waits up to a bounded timeout until the database accepts connections, and records the deployment as running

#### Scenario: Install local deployment times out

- **WHEN** a user runs `exasol install local` and the database does not become ready within the bounded timeout
- **THEN** the launcher fails the command rather than waiting indefinitely

#### Scenario: Reject unsupported local platform

- **WHEN** a user runs `exasol install local` on an unsupported operating system or architecture
- **THEN** the launcher fails before starting a VM and explains that the Exasol Local deployment is only supported on macOS Apple Silicon

### Requirement: Local lifecycle commands

The system SHALL support standard lifecycle commands for local deployments.

#### Scenario: Stop local deployment

- **WHEN** a local deployment is running and the user runs `exasol stop`
- **THEN** the launcher stops the Exasol Local VM and records the deployment as stopped

#### Scenario: Start local deployment

- **WHEN** a local deployment is stopped and the user runs `exasol start`
- **THEN** the launcher starts the Exasol Local VM, waits up to a bounded timeout until the database accepts connections, refreshes connection artifacts, and records the deployment as running

#### Scenario: Start local deployment times out

- **WHEN** a local deployment is stopped, the user runs `exasol start`, and the database does not become ready within the bounded timeout
- **THEN** the launcher fails the command rather than waiting indefinitely

#### Scenario: Destroy local deployment

- **WHEN** a user runs `exasol destroy` for a local deployment
- **THEN** the launcher stops the Exasol Local VM if needed, deletes the local VM disk/data and launcher-owned runtime artifacts, removes connection artifacts, and records the deployment as initialized

### Requirement: Local SQL connection

The system SHALL allow `exasol connect` to connect to the Exasol Local database using the local deployment artifacts.

#### Scenario: Connect to local database

- **WHEN** a local deployment is running and the user runs `exasol connect`
- **THEN** the launcher connects to the Exasol Local database through the loopback database endpoint using the stored local credentials, within a bounded timeout

#### Scenario: Local certificate validation mode

- **WHEN** the launcher creates connection metadata for a local deployment without a stable database certificate fingerprint
- **THEN** the metadata marks certificate validation as insecure for that local deployment
