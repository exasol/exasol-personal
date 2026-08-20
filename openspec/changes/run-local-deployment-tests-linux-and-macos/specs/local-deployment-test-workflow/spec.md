## Purpose

Defines how maintainers manually select and run real local deployment tests across the enabled Linux AMD64 and macOS ARM64 CI platforms.

## ADDED Requirements

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

### Requirement: Cloud and local selection coexist
The workflow SHALL preserve existing cloud test-plan behavior while applying the OS selector to local rows.

#### Scenario: Combined suite selects all rows
- **WHEN** a maintainer dispatches the combined suite with the all-OS selector
- **THEN** the workflow creates the enabled cloud jobs and both enabled local jobs

#### Scenario: Combined suite selects Windows
- **WHEN** a maintainer dispatches the combined suite with the Windows selector
- **THEN** matching cloud jobs run and no local deployment job is created
