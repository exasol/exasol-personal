## MODIFIED Requirements

### Requirement: Exasol Local deployment preset

The system SHALL provide the standard local deployment option through a VM-backed Podman installation on macOS Apple Silicon, and through host Docker or host Podman on Linux AMD64, Linux ARM64, and Windows AMD64.

#### Scenario: Install local deployment

- **WHEN** a user runs `exasol install local` in an empty deployment directory on macOS Apple Silicon
- **THEN** the launcher initializes and starts the VM with application forwards, installs Nano through guest Podman, waits up to a bounded timeout until the database accepts connections, and records the deployment as running

#### Scenario: Install local deployment on Linux

- **WHEN** a user runs `exasol install local` in an empty deployment directory on Linux AMD64 or ARM64
- **THEN** the launcher initializes the deployment directory, starts Nano through the container engine the deployment selects, waits up to a bounded timeout until the database accepts connections, and records the deployment as running

#### Scenario: Install local deployment on Windows

- **WHEN** a user runs `exasol install local` in an empty deployment directory on Windows AMD64
- **THEN** the launcher initializes the deployment directory, starts Nano through host Docker when the deployment selects Docker, or otherwise prepares Podman and its default machine and starts Nano through host Podman, waits up to a bounded timeout until the database accepts connections, and records the deployment as running

#### Scenario: Install local deployment times out

- **WHEN** a user runs `exasol install local` and the database does not become ready within the bounded timeout
- **THEN** the launcher fails the command rather than waiting indefinitely

#### Scenario: Reject unsupported local platform

- **WHEN** a user runs `exasol install local` on another operating system or architecture
- **THEN** the launcher fails before starting a runtime and lists the supported local platforms

### Requirement: Linux local status and health reflect the published database endpoint
The system SHALL derive Linux runtime status from the exact deployment container in its container engine, recover missing in-memory endpoints from deployment information, and probe the published database port for health.

#### Scenario: Runtime memory has no endpoints
- **WHEN** Linux local status or health is requested after process restart and deployment information contains the database endpoint
- **THEN** the runtime recovers that endpoint and uses it for status and health reporting

#### Scenario: Published database endpoint accepts connections
- **WHEN** the exact deployment container is running and the published database port accepts a connection
- **THEN** local health is healthy

#### Scenario: Linux database is unreachable
- **WHEN** the published Linux database endpoint cannot be reached
- **THEN** diagnostics describe the Linux host-container failure without macOS host-to-VM guidance

### Requirement: Local startup crosses explicit durability boundaries

The system SHALL synchronize local runtime storage on Linux and macOS after preparing container images and SHALL flush Nano startup state after the database becomes ready, without automatically repairing or resetting the container engine's storage.

#### Scenario: Images are synchronized before container startup

- **WHEN** the launcher has loaded, tagged, materialized, and pruned the images required by a local deployment
- **THEN** it synchronizes the execution environment before creating the Nano container

#### Scenario: Nano startup state is synchronized before startup succeeds

- **WHEN** a local database accepts connections during install or start
- **THEN** the launcher synchronizes the execution environment before reporting startup success

#### Scenario: Storage synchronization fails

- **WHEN** either startup synchronization fails
- **THEN** the launcher fails startup without automatically repairing or resetting the container engine's storage
