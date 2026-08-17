## ADDED Requirements

### Requirement: macOS local VM memory default
The system SHALL default local deployment VM memory on macOS to approximately 50% of total host memory when the user has not configured local VM memory explicitly.

#### Scenario: Default local VM memory on macOS
- **WHEN** a user initializes or starts a local deployment on macOS without setting `memory_mb`
- **THEN** the launcher uses a default local VM memory value of approximately 50% of total host memory

#### Scenario: Explicit local VM memory overrides macOS default
- **WHEN** a user configures `memory_mb` for a local deployment on macOS
- **THEN** the launcher uses the configured value instead of the computed default

### Requirement: minimum configured local VM memory
The system SHALL reject user-configured local deployment memory below 4096 MB.

#### Scenario: Configured local VM memory below minimum
- **WHEN** a user configures `memory_mb` below 4096 for a local deployment
- **THEN** the launcher fails before using the configuration and explains that `memory_mb` must be at least 4096 MB

### Requirement: minimum host memory for macOS local deployment
The system SHALL fail local deployment on macOS when detected host memory is below 8192 MB.

#### Scenario: Host memory below minimum
- **WHEN** the launcher detects host memory below 8192 MB for a macOS local deployment
- **THEN** the launcher fails before starting the local deployment and explains the required and detected host memory
