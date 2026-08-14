# windows-host-runtime-environment Specification

## Purpose
Defines how Windows hosts become ready to run local deployments without requiring users to configure Podman and its backing machine manually.

## Requirements
### Requirement: Windows Podman availability is ensured with explicit approval
The system SHALL reuse an available Podman installation, SHALL refresh the launcher process PATH from registered Windows environment values before deciding Podman is absent, and SHALL require approval before installing Podman through Windows Package Manager.

#### Scenario: Podman is already available
- **WHEN** Windows Podman can report its version
- **THEN** preparation continues without prompting or invoking Windows Package Manager

#### Scenario: Podman is installed but absent from the process PATH
- **WHEN** Podman becomes available after refreshing PATH from registered machine and user values
- **THEN** preparation continues without reinstalling Podman

#### Scenario: Interactive Podman installation is approved
- **WHEN** Podman remains unavailable and the user approves the displayed Windows Package Manager command
- **THEN** the launcher installs the exact `RedHat.Podman` package from the `winget` source, refreshes PATH, and verifies Podman before continuing

#### Scenario: Podman installation is not approved
- **WHEN** Podman is unavailable and approval is declined or cannot be obtained non-interactively
- **THEN** preparation fails before changing deployment workflow state and explains how to approve or perform the setup manually

### Requirement: Windows default Podman machine is ready and rootful
The system SHALL use the default Podman machine, SHALL create it rootful when absent, SHALL start it when stopped, and SHALL require approval before converting an existing rootless machine.

#### Scenario: Default machine is absent
- **WHEN** Podman is available but its default machine does not exist
- **THEN** the launcher initializes that machine rootful with a 40 GB disk and starts it

#### Scenario: Rootful default machine is stopped
- **WHEN** the default machine is rootful but not running
- **THEN** the launcher starts it without requesting approval

#### Scenario: Rootless conversion is approved
- **WHEN** the default machine is rootless and the user approves conversion
- **THEN** the launcher stops it when necessary, sets it rootful, and starts it

#### Scenario: Rootless conversion is not approved
- **WHEN** the default machine is rootless and approval is declined or unavailable
- **THEN** preparation fails before changing deployment workflow state and leaves the machine mode unchanged

### Requirement: Windows host preparation is retry-safe
The system SHALL make repeated successful preparation calls safe and SHALL preserve actionable errors for partial prerequisite failures.

#### Scenario: Prepared host is checked again
- **WHEN** Podman and the rootful default machine are already ready
- **THEN** preparation performs only readiness checks and makes no host changes

#### Scenario: A prerequisite command fails
- **WHEN** Windows Package Manager or a Podman-machine command fails
- **THEN** the launcher returns the causal command error and a later deploy or start can retry preparation
