## RENAMED Requirements

- FROM: `### Requirement: Podman lifecycle failures are reported`
- TO: `### Requirement: Container engine lifecycle failures are reported`

- FROM: `### Requirement: Podman startup failures include best-effort diagnostics`
- TO: `### Requirement: Container startup failures include best-effort diagnostics`

- FROM: `### Requirement: Podman image loading uses runtime artifact paths`
- TO: `### Requirement: Nano image loading uses runtime artifact paths`

- FROM: `### Requirement: Podman installation uses its execution environment`
- TO: `### Requirement: Nano installation uses its execution environment`

## MODIFIED Requirements

### Requirement: Linux host runtime starts a persistent Nano container
The system SHALL start an Exasol Nano container through the deployment's container engine when a local deployment uses the Linux host runtime, and SHALL keep the database data in the deployment's local runtime directory.

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
- **THEN** the start operation fails before the container engine is invoked

### Requirement: Linux host containers remain unlimited by VM sizing
The system SHALL NOT apply VM CPU, memory, or data-size settings as container resource limits when using the Linux host runtime.

#### Scenario: VM sizing fields are present
- **WHEN** VM CPU, memory, or data-size values exist in shared configuration state and the Linux host runtime starts the container
- **THEN** the container engine invocation contains no limits derived from those values

### Requirement: Container engine lifecycle failures are reported
The system SHALL stop the start sequence at the failing container engine operation and return an error that identifies the failed lifecycle step.

#### Scenario: Image loading fails
- **WHEN** the container engine fails to load the Nano image
- **THEN** the start operation returns an image-loading error and does not attempt to run the image

#### Scenario: Loaded image cannot be identified
- **WHEN** the container engine reports a successful load but its documented output does not identify the loaded image
- **THEN** the start operation fails before running a container

### Requirement: Linux host container cleanup is idempotent
The system SHALL tolerate cleanup when the deployment's container is absent.

#### Scenario: Stop a deployment
- **WHEN** the deployment is stopped whether or not its named container exists
- **THEN** the disposable container is absent and the persistent data directory remains

#### Scenario: Destroy a deployment
- **WHEN** the deployment is destroyed whether or not its named container exists
- **THEN** the disposable container is absent and the deployment's local runtime data is removed

### Requirement: Container startup failures include best-effort diagnostics
The system SHALL emit container engine environment, container-list, deployment-container inspection, and deployment-container log diagnostics when image materialization, container recreation, or startup fails, without replacing the original failure.

#### Scenario: Startup fails and diagnostics partly fail
- **WHEN** startup fails and one or more diagnostic commands also fail
- **THEN** the original startup error is returned and all remaining diagnostics are attempted

### Requirement: Nano image loading uses runtime artifact paths
The system SHALL load a Nano image using the container engine's `load -i` with the runtime path supplied for the materialized image and SHALL run the exact image reference reported by the load without creating a deployment-specific image tag.

#### Scenario: Linux host loads Nano image
- **WHEN** the Linux runtime materializes the embedded Nano image
- **THEN** the runtime path equals the host path and the container engine reads that path through `-i`

#### Scenario: VM-backed runtime loads Nano image
- **WHEN** a VM-backed runtime materializes the embedded Nano image in its shared directory
- **THEN** Podman reads the mapped guest path through `-i` without installation code selecting a transport

#### Scenario: Loaded image is identified by name
- **WHEN** the container engine reports the loaded Nano image as a named image reference
- **THEN** the system runs that exact reference without creating another image tag

#### Scenario: Loaded image is identified by ID
- **WHEN** the container engine reports the loaded Nano image as an image ID
- **THEN** the system runs that exact ID without creating an image tag
