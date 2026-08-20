## ADDED Requirements

### Requirement: Historical local data remains persistent during adoption

The system SHALL preserve Nano data when adopting a historical local container, migrating `/exa` only when it is overlay-backed or mounted from a location other than the deployment's managed persistent data directory.

#### Scenario: Adopt a legacy-named container with managed persistent data

- **WHEN** a historical local container uses a legacy name and its `/exa` mount already originates from the deployment's managed persistent data directory
- **THEN** the launcher adopts the container without copying `/exa` or overwriting the managed persistent data directory

#### Scenario: Adopt a legacy container with data outside the managed directory

- **WHEN** a historical local container's `/exa` data is overlay-backed or mounted from a location other than the deployment's managed persistent data directory
- **THEN** the launcher stages that data into the managed persistent data directory before replacing the historical container

### Requirement: Local startup crosses explicit durability boundaries

The system SHALL synchronize local runtime storage on Linux and macOS after preparing container images and SHALL flush Nano startup state after the database becomes ready, without automatically repairing or resetting the Podman store.

#### Scenario: Images are synchronized before container startup

- **WHEN** the launcher has loaded, tagged, materialized, and pruned the images required by a local deployment
- **THEN** it synchronizes the execution environment before creating the Nano container

#### Scenario: Nano startup state is synchronized before startup succeeds

- **WHEN** a local database accepts connections during install or start
- **THEN** the launcher synchronizes the execution environment before reporting startup success

#### Scenario: Storage synchronization fails

- **WHEN** either startup synchronization fails
- **THEN** the launcher fails startup without automatically repairing or resetting the Podman store
