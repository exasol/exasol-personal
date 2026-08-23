## ADDED Requirements

### Requirement: Windows Podman availability is ensured with explicit approval
The system SHALL reuse an available Podman installation, SHALL refresh the launcher process PATH from registered Windows environment values before deciding Podman is absent, and SHALL require approval before installing Podman through Windows Package Manager.

#### Scenario: Podman is already available
- **WHEN** Podman can be resolved from the launcher process PATH
- **THEN** preparation continues without requesting approval or invoking Windows Package Manager

#### Scenario: Podman is installed but absent from the process PATH
- **WHEN** Podman becomes resolvable after refreshing PATH from registered machine and user values
- **THEN** preparation continues without requesting approval or reinstalling Podman

#### Scenario: Podman installation is approved
- **WHEN** Podman remains unavailable and the displayed Windows Package Manager command is approved
- **THEN** the launcher installs the exact `RedHat.Podman` package from the `winget` source, refreshes PATH, and verifies Podman before continuing

#### Scenario: Podman installation is declined
- **WHEN** Podman is unavailable and the host change is declined
- **THEN** preparation fails without invoking Windows Package Manager

#### Scenario: Windows Package Manager is unavailable
- **WHEN** Podman is unavailable and Windows Package Manager cannot be resolved
- **THEN** preparation fails without requesting approval and explains how to obtain Windows Package Manager or install Podman manually

#### Scenario: Podman remains unavailable after installation
- **WHEN** installation reports success but Podman still cannot be resolved after refreshing PATH
- **THEN** preparation fails and states that Podman is not on PATH

### Requirement: Windows default Podman machine is ready
The system SHALL use Podman's default machine, SHALL initialize it with Podman's default privilege mode when absent, SHALL start it when it is not running, and SHALL NOT change the configuration of an existing machine.

#### Scenario: Default machine is absent
- **WHEN** Podman is available but its default machine does not exist
- **THEN** the launcher initializes that machine with a fixed disk size using Podman's default privilege mode, and starts it

#### Scenario: Default machine is stopped
- **WHEN** the default machine exists but is not running
- **THEN** the launcher starts it without changing its privilege mode

#### Scenario: Default machine is already running
- **WHEN** the default machine is already running
- **THEN** preparation makes no changes to the machine

#### Scenario: Default machine stops between preparation and start
- **WHEN** the default machine is no longer running when the deployment starts
- **THEN** the launcher starts it again without requesting approval

### Requirement: Windows host preparation is retry-safe
The system SHALL make repeated successful preparation calls safe and SHALL preserve actionable errors for partial prerequisite failures.

#### Scenario: Prepared host is checked again
- **WHEN** Podman and the default machine are already ready
- **THEN** preparation performs only readiness checks and makes no host changes

#### Scenario: A prerequisite command fails
- **WHEN** Windows Package Manager or a Podman-machine command fails
- **THEN** the launcher returns the causal command error and a later deploy or start can retry preparation

### Requirement: Windows local storage durability crosses the Podman machine boundary
The system SHALL flush Nano's startup writes inside the Podman machine whose kernel buffers them, because Windows offers no host-side equivalent.

#### Scenario: Database becomes ready on Windows
- **WHEN** a Windows local deployment's database becomes ready after start
- **THEN** the launcher flushes storage inside the Podman machine rather than on the Windows host filesystem

#### Scenario: Flushing inside the Podman machine fails
- **WHEN** the in-machine flush cannot be executed
- **THEN** start fails with the causal error rather than reporting a durable start
