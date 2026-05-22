## ADDED Requirements

### Requirement: Default deployment directory reuse SHALL avoid stale preset deployment
When commands resolve to the default deployment directory, they SHALL validate requested preset identity against initialized local state before deploying resources.

#### Scenario: Same default directory install retry uses same preset state
- **WHEN** a user reruns `exasol install <infra-preset>` outside a recognized deployment directory
- **AND** the default deployment directory is initialized with the same preset identity
- **THEN** the command reuses the default deployment directory
- **AND** the command does not remove local deployment files before deployment retry

#### Scenario: Different default directory preset is refused
- **WHEN** a user runs `exasol install <different-infra-preset>` outside a recognized deployment directory
- **AND** the default deployment directory is initialized with another preset identity
- **THEN** the command fails before deployment
- **AND** the command does not deploy the stale preset from the default deployment directory
- **AND** the command tells the user to run `exasol destroy --remove` before initializing different presets, or `exasol remove` if the cloud resources are already gone

### Requirement: Commands SHALL log the resolved deployment directory and how it was resolved
Whenever a command resolves a deployment directory from a flag, the current working directory, or the default location, the launcher SHALL log the resolved path together with the resolution source so log trails are unambiguous about where the launcher is operating.

#### Scenario: Resolved deployment directory and source are logged
- **WHEN** a command resolves its deployment directory
- **THEN** the launcher emits a structured log entry containing the resolved deployment directory path
- **AND** the log entry contains the resolution source (`explicit`, `current`, or `default`)
- **AND** the log entry is emitted regardless of whether the directory was supplied explicitly, inferred from the current directory, or defaulted
