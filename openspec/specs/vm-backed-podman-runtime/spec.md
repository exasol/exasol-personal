# vm-backed-podman-runtime Specification

## Purpose
Defines how macOS local deployments run the shared Podman installation inside a VM without exposing VM transport details to installation code.
## Requirements
### Requirement: macOS uses a VM-only runner contract
The system SHALL require a VM runner that provides VM lifecycle, labeled forwarding, shared-path exchange, and guest command execution without owning application containers.

#### Scenario: Compatible runner is used
- **WHEN** a macOS local deployment prepares a runner implementing the VM-only contract
- **THEN** the system initializes and starts the VM before invoking the shared Podman installation inside it

#### Scenario: Legacy runner is resolved
- **WHEN** a macOS local deployment resolves a runner that does not implement the VM-only contract
- **THEN** preparation fails before the runner can start a legacy database container

### Requirement: macOS installs Nano through guest Podman
The system SHALL perform Nano image loading, SLC materialization, container creation, status inspection, diagnostics, and cleanup through Podman commands executed inside the VM.

#### Scenario: Fresh macOS deployment starts
- **WHEN** the VM is running and the deployment has no running Nano container
- **THEN** the same persistent Podman installation behavior used by the Linux host runtime runs inside the VM

#### Scenario: macOS deployment stops
- **WHEN** a running macOS deployment is stopped
- **THEN** its disposable Nano container is removed before the VM is stopped and persistent guest data remains

### Requirement: macOS maps staged artifacts into the VM
The system SHALL stage host artifacts below the VM shared directory and provide both host and guest paths to installation behavior.

#### Scenario: Nano image is loaded
- **WHEN** the shared installation loads the embedded Nano image on macOS
- **THEN** it invokes `podman load -i` with the corresponding path below `/mnt/host`

#### Scenario: Custom SLC package is imported
- **WHEN** a configured custom SLC package is available on the macOS host
- **THEN** it is staged in the VM share and imported using its guest path

#### Scenario: Path is outside the VM share
- **WHEN** a host artifact cannot be represented below the VM shared directory
- **THEN** installation fails before a guest command receives an unusable host path

### Requirement: macOS endpoints come from labeled VM forwards
The system SHALL request labeled VM forwards for application services before VM startup and derive connection endpoints from the effective mappings reported by the runner.

#### Scenario: Dynamic database host port is requested
- **WHEN** the configured database host port is zero
- **THEN** the runtime requests a `db` forward to guest port `8563` and publishes the assigned loopback port as the database endpoint

#### Scenario: Admin UI is enabled
- **WHEN** the local installation exposes its Admin UI guest port
- **THEN** the runtime requests a labeled UI forward and publishes the effective loopback endpoint

#### Scenario: Requested host port is occupied
- **WHEN** the runner cannot bind a requested nonzero service port
- **THEN** local startup fails without substituting another host port

### Requirement: legacy runner database assets are retired safely
The system SHALL remove obsolete host-shared runner database payloads and SHALL preserve guest database data required by the shared Podman installation.

#### Scenario: Existing deployment contains legacy shared payloads
- **WHEN** a deployment starts with VM-only runner support
- **THEN** obsolete shared Nano payload and runner configuration files are removed before installation

#### Scenario: Existing guest data is present
- **WHEN** a deployment created by an earlier runner has persistent database data in the VM data disk
- **THEN** the shared installation reuses or migrates that data without deleting it during normal start
