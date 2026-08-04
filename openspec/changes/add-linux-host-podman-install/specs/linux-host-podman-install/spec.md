## Purpose

Defines the minimal persistent lifecycle for running an Exasol Nano database directly through Podman on a Linux host.

## ADDED Requirements

### Requirement: Linux host starts a persistent Nano container
The system SHALL start the deployment's Nano database as a named Podman container with deployment-owned data mounted at `/exa`.

#### Scenario: Fresh database start
- **WHEN** a Linux host deployment is started without an existing Nano configuration
- **THEN** the system loads and tags the configured Nano image, creates the persistent data directory, and starts Nano with the first-deployment initialization parameters

#### Scenario: Existing database start
- **WHEN** a Linux host deployment is started with an existing Nano configuration in its persistent data directory
- **THEN** the system starts Nano using the existing configuration without reapplying first-deployment parameters

#### Scenario: Database is already running
- **WHEN** the deployment's named container is already running
- **THEN** the start operation succeeds without loading an image or replacing the running container

### Requirement: Linux host publishes the configured database endpoint
The system SHALL publish Nano's fixed container database port through the configured host database port.

#### Scenario: Default database port
- **WHEN** no database host port override is configured
- **THEN** the system publishes host port `8563` to container port `8563`

#### Scenario: Overridden database host port
- **WHEN** `db:<port>` is configured for the local runtime
- **THEN** the system publishes the configured host port to container port `8563`

#### Scenario: Invalid database host port
- **WHEN** the database host port configuration is malformed, duplicated, or outside the valid TCP port range
- **THEN** the start operation fails before starting the container

### Requirement: Host containers remain unlimited
The system SHALL NOT apply VM CPU, memory, or data-size settings as Podman resource limits to a plain Linux host container.

#### Scenario: VM sizing is present
- **WHEN** a Linux host deployment is started with VM sizing fields in the shared runtime input
- **THEN** the Podman command does not contain CPU, memory, or storage quota options derived from those fields

### Requirement: Podman lifecycle failures are reported
The system SHALL stop the startup sequence and report an error when resolving, loading, identifying, tagging, or running the Nano image fails.

#### Scenario: Image load fails
- **WHEN** Podman cannot load the Nano image archive
- **THEN** the start operation fails without tagging or running a container

#### Scenario: Loaded image cannot be identified
- **WHEN** a successful image load does not report a usable image reference
- **THEN** the start operation fails without running a container

### Requirement: Container cleanup is idempotent
The system SHALL allow stop and destroy operations to succeed when the deployment container is already absent.

#### Scenario: Stop removes the disposable container
- **WHEN** a Linux host deployment is stopped
- **THEN** the named container is forcibly removed while the persistent data directory remains

#### Scenario: Destroy removes deployment runtime data
- **WHEN** a Linux host deployment is destroyed
- **THEN** the named container and deployment-owned runtime data are removed
