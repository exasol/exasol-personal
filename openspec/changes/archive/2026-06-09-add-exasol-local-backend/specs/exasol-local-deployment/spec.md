## ADDED Requirements

### Requirement: Exasol Local deployment preset

The system SHALL provide a local deployment option for macOS Apple Silicon that manages the Exasol Local VM through the standard launcher workflow.

#### Scenario: Install local deployment

- **WHEN** a user runs `exasol install local` in an empty deployment directory on supported macOS Apple Silicon
- **THEN** the launcher initializes the deployment directory, starts the Exasol Local VM, and records the deployment as running

#### Scenario: Reject unsupported local platform

- **WHEN** a user runs `exasol install local` on an unsupported operating system or architecture
- **THEN** the launcher fails before starting a VM and explains that the Exasol Local deployment is only supported on macOS Apple Silicon

### Requirement: Launcher-owned local runtime

The system SHALL own the local VM runtime files, VM disk/data, and managed deployment share inside the deployment directory.

#### Scenario: Managed share is prepared

- **WHEN** the launcher initializes a local deployment
- **THEN** it creates a launcher-managed share for guest coordination and SSH key import

#### Scenario: Embedded runner is staged

- **WHEN** the launcher initializes or starts a local deployment
- **THEN** it writes the embedded macOS Exasol Local runner into the launcher-owned runtime directory before invoking it

#### Scenario: Runner VM sizing is prepared

- **WHEN** the launcher initializes or starts a local deployment
- **THEN** it exposes VM CPU, VM memory, and Exasol Local data disk sizing through the runner start command

#### Scenario: User shares are not exposed

- **WHEN** the user initializes or starts a local deployment
- **THEN** the launcher does not require or expose user-configurable shared folder settings

### Requirement: Local deployment artifacts

The system SHALL write standard launcher artifacts for local deployments so existing information, connection, status, and shell commands can operate on the deployment directory.

#### Scenario: Connection artifacts are written after startup

- **WHEN** the Exasol Local VM starts successfully
- **THEN** the launcher writes `deployment.json`, `secrets.json`, and connection instructions with loopback connection details

#### Scenario: Local credentials are available

- **WHEN** the launcher writes secrets for a local deployment
- **THEN** `secrets.json` contains database credentials for user `sys` with password `exasol`

#### Scenario: Forwarded ports are refreshed

- **WHEN** a local deployment is started after being stopped
- **THEN** the launcher refreshes `deployment.json` with the current forwarded SSH, database, and UI ports

### Requirement: Local SQL connection

The system SHALL allow `exasol connect` to connect to the Exasol Local database using the local deployment artifacts.

#### Scenario: Connect to local database

- **WHEN** a local deployment is running and the user runs `exasol connect`
- **THEN** the launcher connects to the Exasol Local database through the loopback database endpoint using the stored local credentials

#### Scenario: Local certificate validation mode

- **WHEN** the launcher creates connection metadata for a local deployment without a stable database certificate fingerprint
- **THEN** the metadata marks certificate validation as insecure for that local deployment

### Requirement: Local lifecycle commands

The system SHALL support standard lifecycle commands for local deployments.

#### Scenario: Stop local deployment

- **WHEN** a local deployment is running and the user runs `exasol stop`
- **THEN** the launcher stops the Exasol Local VM and records the deployment as stopped

#### Scenario: Start local deployment

- **WHEN** a local deployment is stopped and the user runs `exasol start`
- **THEN** the launcher starts the Exasol Local VM, waits until the database accepts connections, refreshes connection artifacts, and records the deployment as running

#### Scenario: Destroy local deployment

- **WHEN** a user runs `exasol destroy` for a local deployment
- **THEN** the launcher stops the Exasol Local VM if needed, deletes the local VM disk/data and launcher-owned runtime artifacts, removes connection artifacts, and records the deployment as initialized

### Requirement: Local shell access

The system SHALL provide shell access for local deployments through the existing shell commands.

#### Scenario: Host shell

- **WHEN** a local deployment is running and the user runs `exasol shell host`
- **THEN** the launcher opens an interactive SSH shell to the Exasol Local VM through the forwarded SSH endpoint

#### Scenario: Container shell

- **WHEN** a local deployment is running and the user runs `exasol shell container`
- **THEN** the launcher opens an interactive shell inside the Exasol Local database container
