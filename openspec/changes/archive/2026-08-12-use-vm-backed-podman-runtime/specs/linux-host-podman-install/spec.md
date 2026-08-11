## ADDED Requirements

### Requirement: Podman image loading uses runtime artifact paths
The system SHALL load a Nano image using `podman load -i` with the runtime path supplied for the materialized image.

#### Scenario: Linux host loads Nano image
- **WHEN** the Linux runtime materializes the embedded Nano image
- **THEN** the runtime path equals the host path and Podman reads that path through `-i`

#### Scenario: VM-backed runtime loads Nano image
- **WHEN** a VM-backed runtime materializes the embedded Nano image in its shared directory
- **THEN** Podman reads the mapped guest path through `-i` without installation code selecting a transport

### Requirement: Podman installation uses its execution environment
The system SHALL perform command execution and installation-owned filesystem operations through the selected runtime execution environment.

#### Scenario: Linux host installation executes
- **WHEN** the selected environment is the Linux host
- **THEN** commands and filesystem operations execute directly with existing Linux behavior

#### Scenario: VM-backed installation executes
- **WHEN** the selected environment is the macOS VM
- **THEN** commands and guest filesystem operations execute through the VM runner

### Requirement: custom SLC imports use runtime paths
The system SHALL pass runtime-visible paths rather than host-only paths when importing custom SLC packages.

#### Scenario: Custom package is staged for a VM
- **WHEN** a custom SLC package must be imported by guest Podman
- **THEN** the importer receives the mapped guest path while host-side staging retains the corresponding host path
