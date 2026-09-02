# deployment-status-timeout Specification

## Purpose
Ensure `exasol status` completes in bounded time by applying a default timeout and allowing callers to configure it.
## Requirements
### Requirement: Deployment status checks are time-bounded
`exasol status` SHALL apply a five-second timeout to the complete status operation by default and SHALL allow users to select another positive integer number of seconds with `--timeout`.

#### Scenario: Default timeout bounds a status check
- **WHEN** a user runs `exasol status` without `--timeout`
- **AND** the status operation does not complete within five seconds
- **THEN** the command stops waiting

#### Scenario: Explicit timeout bounds a status check
- **WHEN** a user runs `exasol status --timeout <seconds>` with a positive integer
- **AND** the status operation does not complete within that number of seconds
- **THEN** the command stops waiting at the selected bound

#### Scenario: Non-positive timeout is rejected
- **WHEN** a user runs `exasol status --timeout <seconds>` with a zero or negative integer
- **THEN** the command reports that the timeout must be positive without starting the status operation
