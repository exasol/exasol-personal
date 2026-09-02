## ADDED Requirements

### Requirement: Local readiness wait is short and diagnostic
The system SHALL check local database readiness every one to two seconds and SHALL fail a local install or start operation within 30 seconds when the database does not accept connections. When the local runtime identifies a network-wide reachability problem, the failure SHALL retain the most recent database connection error and include the runtime's actionable reachability guidance.

#### Scenario: Local database becomes ready
- **WHEN** a local database accepts connections during install or start
- **THEN** the launcher completes startup without an unnecessary readiness delay

#### Scenario: Local database does not become ready
- **WHEN** a local database does not accept connections during install or start
- **THEN** the launcher fails the operation within 30 seconds and reports the most recent connection failure

#### Scenario: Local network path is blocked
- **WHEN** a local readiness wait ends and the runtime identifies a network-wide reachability problem
- **THEN** the launcher reports actionable reachability guidance together with the most recent connection failure

#### Scenario: Cloud database readiness is unchanged
- **WHEN** a cloud deployment waits for its database to accept connections
- **THEN** the launcher continues to use the cloud readiness timing behavior
