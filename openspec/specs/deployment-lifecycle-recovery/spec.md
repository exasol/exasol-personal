# deployment-lifecycle-recovery Specification

## Purpose
TBD - created by archiving change improve-local-reachability-diagnostics. Update Purpose after archive.
## Requirements
### Requirement: Start and Stop recover to an interrupted state on failure

The system SHALL transition a deployment out of `operation_in_progress` when the underlying `start` or `stop` operation fails, rather than leaving it permanently stuck reporting an in-progress operation.

#### Scenario: Start fails after the backend call errors

- **WHEN** `exasol start` sets the deployment to `operation_in_progress` and the backend's start operation subsequently fails for any reason
- **THEN** the deployment transitions to an interrupted state naming the failed operation, rather than remaining in `operation_in_progress`

#### Scenario: Stop fails after the backend call errors

- **WHEN** `exasol stop` sets the deployment to `operation_in_progress` and the backend's stop operation subsequently fails for any reason
- **THEN** the deployment transitions to an interrupted state naming the failed operation, rather than remaining in `operation_in_progress`

#### Scenario: Recovery is possible after interruption

- **WHEN** a deployment is in an interrupted state following a failed start or stop
- **THEN** `exasol status` reports actionable recovery guidance, and a subsequent `start` or `stop` is permitted to retry the operation
