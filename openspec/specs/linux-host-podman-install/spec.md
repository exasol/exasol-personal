# linux-host-podman-install Specification

## Purpose

Defines the minimal persistent lifecycle for running an Exasol Nano database directly through Podman on a Linux host.

## Requirements

### Requirement: Linux host runtime starts a persistent Nano container
The system SHALL start an Exasol Nano container through Podman when a local deployment uses the Linux host runtime, and SHALL keep the database data in the deployment's local runtime directory.

#### Scenario: Start a fresh deployment
- **WHEN** the deployment data directory does not contain an initialized database
- **THEN** the system loads the configured Nano image, starts the deployment's named container, mounts the persistent data directory at `/exa`, and passes the initial database parameters

#### Scenario: Start an existing deployment
- **WHEN** the deployment data directory already contains an initialized database
- **THEN** the system starts the deployment's named container with the persistent data directory and does not pass first-start database parameters

#### Scenario: Start an already running deployment
- **WHEN** the deployment's named container is already running
- **THEN** the system returns success without loading the image or replacing the container

### Requirement: Linux host runtime publishes the configured database endpoint
The system SHALL publish the deployment's configured host database port to the Nano container's fixed database port.

#### Scenario: Use the default database port
- **WHEN** no database port override is configured
- **THEN** the system publishes host port `8563` to container port `8563`

#### Scenario: Use a database port override
- **WHEN** the deployment configures `db:<host-port>`
- **THEN** the system publishes `<host-port>` on the host to port `8563` in the container

#### Scenario: Reject an invalid database port mapping
- **WHEN** the configured database port mapping cannot be represented as a single host port and container port
- **THEN** the start operation fails before Podman is invoked

### Requirement: Linux host containers remain unlimited by VM sizing
The system SHALL NOT apply VM CPU, memory, or data-size settings as Podman container resource limits when using the Linux host runtime.

#### Scenario: VM sizing fields are present
- **WHEN** VM CPU, memory, or data-size values exist in shared configuration state and the Linux host runtime starts the container
- **THEN** the Podman invocation contains no limits derived from those values

### Requirement: Podman lifecycle failures are reported
The system SHALL stop the start sequence at the failing Podman operation and return an error that identifies the failed lifecycle step.

#### Scenario: Image loading fails
- **WHEN** Podman fails to load the Nano image
- **THEN** the start operation returns an image-loading error and does not attempt to tag or run the image

#### Scenario: Loaded image cannot be identified
- **WHEN** Podman reports a successful load but its documented output does not identify the loaded image
- **THEN** the start operation fails before tagging or running a container

### Requirement: Linux host container cleanup is idempotent
The system SHALL tolerate cleanup when the deployment's Podman container is absent.

#### Scenario: Stop a deployment
- **WHEN** the deployment is stopped whether or not its named container exists
- **THEN** the disposable container is absent and the persistent data directory remains

#### Scenario: Destroy a deployment
- **WHEN** the deployment is destroyed whether or not its named container exists
- **THEN** the disposable container is absent and the deployment's local runtime data is removed
