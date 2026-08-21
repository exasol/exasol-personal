## MODIFIED Requirements

### Requirement: Podman lifecycle failures are reported
The system SHALL stop the start sequence at the failing Podman operation and return an error that identifies the failed lifecycle step.

#### Scenario: Image loading fails
- **WHEN** Podman fails to load the Nano image
- **THEN** the start operation returns an image-loading error and does not attempt to run the image

#### Scenario: Loaded image cannot be identified
- **WHEN** Podman reports a successful load but its documented output does not identify the loaded image
- **THEN** the start operation fails before running a container

### Requirement: Podman image loading uses runtime artifact paths
The system SHALL load a Nano image using `podman load -i` with the runtime path supplied for the materialized image and SHALL run the exact image reference reported by the load without creating a deployment-specific image tag.

#### Scenario: Linux host loads Nano image
- **WHEN** the Linux runtime materializes the embedded Nano image
- **THEN** the runtime path equals the host path and Podman reads that path through `-i`

#### Scenario: VM-backed runtime loads Nano image
- **WHEN** a VM-backed runtime materializes the embedded Nano image in its shared directory
- **THEN** Podman reads the mapped guest path through `-i` without installation code selecting a transport

#### Scenario: Loaded image is identified by name
- **WHEN** Podman reports the loaded Nano image as a named image reference
- **THEN** the system runs that exact reference without creating another image tag

#### Scenario: Loaded image is identified by ID
- **WHEN** Podman reports the loaded Nano image as an image ID
- **THEN** the system runs that exact ID without creating an image tag
