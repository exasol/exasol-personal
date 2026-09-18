## ADDED Requirements

### Requirement: Local status reports a stopped runtime environment
The system SHALL report a local deployment as stopped when the environment hosting its
container is not running, and SHALL resolve that status without starting the environment.

#### Scenario: Windows Podman machine is not running

- **WHEN** a local deployment's recorded state is running, the Podman machine is not running,
  and the user runs `exasol status`
- **THEN** the command reports status `stopped` with guidance to run `start` to restart or
  `destroy` to delete resources
- **AND** the Podman machine remains stopped

#### Scenario: Stopped environment is reported within the status timeout

- **WHEN** `exasol status` resolves a local deployment whose environment is not running under
  the default timeout
- **THEN** the command completes within that timeout

#### Scenario: Stopped environment is reported in machine-readable output

- **WHEN** a user runs `exasol status --json` for a local deployment whose environment is not
  running
- **THEN** the output reports the same status value and message as the text output
