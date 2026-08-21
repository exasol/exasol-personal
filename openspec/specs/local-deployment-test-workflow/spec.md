# local-deployment-test-workflow Specification

## Purpose
Defines how maintainers manually select and run real local deployment tests across the enabled Linux AMD64 and macOS ARM64 CI platforms.

## Requirements

### Requirement: Local deployment tests cover enabled platforms
The manual deployment-test workflow SHALL provide real local deployment test rows for Linux AMD64 and macOS ARM64, and SHALL NOT create real local deployment rows for Windows or Linux ARM64.

#### Scenario: All local platforms are selected
- **WHEN** a maintainer dispatches the local suite with the all-OS selector
- **THEN** the workflow creates one Linux AMD64 local deployment job and one macOS ARM64 local deployment job

#### Scenario: One supported local platform is selected
- **WHEN** a maintainer dispatches the local suite with the Linux or macOS OS selector
- **THEN** the workflow creates only the matching local deployment job

#### Scenario: An unsupported local platform is selected
- **WHEN** a maintainer dispatches only the local suite with the Windows OS selector
- **THEN** workflow planning fails with an actionable incompatibility message before any deployment job starts

### Requirement: Local jobs use platform-compatible runners
Each local deployment test row SHALL use runner and tool-setup behavior compatible with its selected platform.

#### Scenario: Linux local tests run
- **WHEN** the Linux AMD64 local row is selected
- **THEN** it runs on the GitHub-hosted Ubuntu runner using the managed Python setup path and the local deployment test task

#### Scenario: macOS local tests run
- **WHEN** the macOS ARM64 local row is selected
- **THEN** it runs on the existing self-hosted ARM64 virtualization runner using the system-Python setup path and the local deployment test task

### Requirement: Local test selection follows runtime capabilities
The local deployment test suite SHALL run portable lifecycle coverage on every enabled local runtime, SHALL reserve platform skips for runtime-specific behavior, and SHALL exclude tests that cannot apply to local deployments.

#### Scenario: Portable lifecycle runs on Linux
- **WHEN** the Linux AMD64 local row runs the full local deployment lifecycle
- **THEN** initialization, deployment, query, stop, start, and cleanup execute against the Linux host runtime and the published connection metadata does not advertise shell access

#### Scenario: Non-VM port forwarding resets before the database listens
- **WHEN** a non-VM local runtime's published database port resets a readiness connection during startup
- **THEN** the launcher treats the reset as a transient refused connection and continues database readiness waiting instead of reporting a blocked network path

#### Scenario: Portable lifecycle runs on macOS
- **WHEN** the macOS ARM64 local row runs the full local deployment lifecycle
- **THEN** initialization, deployment, query, stop, start, and cleanup execute against the macOS VM runtime and the published connection metadata reports shell access as supported

#### Scenario: VM-specific behavior is selected on Linux
- **WHEN** the Linux AMD64 local row collects memory-sizing, historical-update, or VM-daemon recovery cases
- **THEN** those cases are explicitly skipped as macOS VM-specific

#### Scenario: A test rejects local deployments
- **WHEN** a test unconditionally skips for the local infrastructure preset
- **THEN** it is not selected by the local deployment test marker

#### Scenario: Optional custom-SLC input is absent
- **WHEN** neither supported custom-SLC source variable is supplied for a local workflow dispatch
- **THEN** the custom-SLC cases may skip without affecting the remaining platform coverage

### Requirement: Cloud and local selection coexist
The workflow SHALL preserve existing cloud test-plan behavior while applying the OS selector to local rows.

#### Scenario: Combined suite selects all rows
- **WHEN** a maintainer dispatches the combined suite with the all-OS selector
- **THEN** the workflow creates the enabled cloud jobs and both enabled local jobs

#### Scenario: Combined suite selects Windows
- **WHEN** a maintainer dispatches the combined suite with the Windows selector
- **THEN** matching cloud jobs run and no local deployment job is created
