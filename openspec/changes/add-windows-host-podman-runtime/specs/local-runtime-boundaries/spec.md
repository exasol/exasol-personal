## MODIFIED Requirements

### Requirement: Local runtime implementation is isolated
The system SHALL keep macOS VM mechanics and direct-host mechanics outside the deployment workflow package.

#### Scenario: Local runner lifecycle is delegated
- **WHEN** the local backend deploys, starts, stops, destroys, reports status, checks health, executes a command, maps an artifact path, or opens a shell
- **THEN** it delegates platform-specific behavior to the selected local runtime component

#### Scenario: Deployment workflow remains the artifact owner
- **WHEN** the selected local runtime reports endpoint state after deploy or start
- **THEN** the local backend maps that state into launcher deployment artifacts and workflow behavior

#### Scenario: Platform prerequisites are contributed per platform
- **WHEN** a direct-host runtime prepares its host, re-checks prerequisites at start, or executes an installation command
- **THEN** the selected platform contributes that behavior to one shared host runtime rather than the runtime branching on the platform

### Requirement: Local runtime selection is centralized
The system SHALL use one platform-selection policy for backend creation, status reconciliation, SLC restart decisions, connection diagnostics, and local diagnostics.

#### Scenario: Supported platform is selected
- **WHEN** the host is macOS Apple Silicon, Linux AMD64, Linux ARM64, or Windows AMD64
- **THEN** every local workflow selects the same corresponding VM or host runtime

#### Scenario: Unsupported platform is selected
- **WHEN** the host is any other operating-system and architecture pair
- **THEN** every local workflow rejects it consistently

#### Scenario: Platform-specific guidance is required
- **WHEN** a caller needs guidance that differs between direct-host platforms
- **THEN** it selects that guidance from the runtime's reported platform rather than from the runtime's concrete type

## ADDED Requirements

### Requirement: Host preparation precedes recorded operation state
The system SHALL satisfy host prerequisites before recording that a deployment operation is in progress.

#### Scenario: Preparation fails during deploy
- **WHEN** host preparation fails or is declined while deploying an initialized deployment
- **THEN** the deployment remains initialized and the operation can be retried

#### Scenario: Preparation fails during start
- **WHEN** host preparation fails or is declined while starting a stopped deployment
- **THEN** the deployment remains stopped and the operation can be retried

#### Scenario: Cloud deployment is prepared
- **WHEN** a tofu-backed cloud deployment is deployed or started
- **THEN** preparation makes no host changes and the workflow proceeds unchanged

### Requirement: Host change approval is a command-layer decision
The system SHALL require explicit approval for host-mutating preparation steps, SHALL present the exact commands to be run, and SHALL keep terminal detection and prompt presentation outside the runtime.

#### Scenario: Runtime requests a host change
- **WHEN** a runtime needs to apply a host-mutating step
- **THEN** it declares the change kind and the exact commands it intends to run, and applies the step only if the command layer approves

#### Scenario: Interactive approval is requested
- **WHEN** a host change requires approval and the command can prompt
- **THEN** the launcher displays the change and the exact commands, and applies the change only on explicit confirmation

#### Scenario: Approval is requested without prompting
- **WHEN** a host change requires approval and unattended approval was requested
- **THEN** the launcher applies the change without prompting

#### Scenario: Approval cannot be obtained
- **WHEN** a host change requires approval, unattended approval was not requested, and the command cannot prompt
- **THEN** preparation fails and explains how to approve the change or perform the setup manually

#### Scenario: No approver is available
- **WHEN** a host change requires approval and no approval policy was supplied
- **THEN** the change is denied rather than applied

### Requirement: Preparation progress is reported independently of verbose output
The system SHALL report host preparation progress regardless of whether optional subprocess output is enabled.

#### Scenario: Long preparation step runs without verbose output
- **WHEN** preparation installs a container runtime or creates a Podman machine and optional subprocess output is disabled
- **THEN** the launcher still reports that the step is running

#### Scenario: Machine-readable lifecycle output is requested
- **WHEN** preparation reports progress during a command that emits machine-readable lifecycle output
- **THEN** progress does not contaminate that output
