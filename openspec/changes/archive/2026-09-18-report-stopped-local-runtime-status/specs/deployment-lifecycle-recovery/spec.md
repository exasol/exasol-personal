## ADDED Requirements

### Requirement: Unreachable database status carries recovery guidance
The system SHALL accompany an unreachable-database status with guidance the user can act on.

#### Scenario: Database is unreachable while the local runtime is running

- **WHEN** a deployment's local runtime is running, its database does not accept connections,
  and the user runs `exasol status`
- **THEN** the command reports status `database_connection_failed` together with a message
  stating how to recover
