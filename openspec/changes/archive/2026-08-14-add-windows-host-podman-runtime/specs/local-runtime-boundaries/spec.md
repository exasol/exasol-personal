## MODIFIED Requirements

### Requirement: Local runtime implementation is isolated
The system SHALL keep VM mechanics and platform host-environment preparation outside the deployment workflow while sharing host container lifecycle behavior across compatible platforms.

#### Scenario: Local runner lifecycle is delegated
- **WHEN** the local backend prepares, deploys, starts, stops, destroys, reports status, checks health, executes a command, maps an artifact path, or opens a shell
- **THEN** it delegates platform-specific behavior to the selected local runtime component and, where applicable, its host-environment preparation implementation

#### Scenario: Deployment workflow remains the artifact owner
- **WHEN** the selected local runtime reports endpoint state after deploy or start
- **THEN** the local backend maps that state into launcher deployment artifacts and workflow behavior

### Requirement: Local runtime selection is centralized
The system SHALL use one platform-selection policy for backend creation, status reconciliation, SLC restart decisions, connection diagnostics, and local diagnostics.

#### Scenario: Supported platform is selected
- **WHEN** the host is macOS Apple Silicon, Linux AMD64, Linux ARM64, or Windows AMD64
- **THEN** every local workflow selects the same corresponding VM or host runtime environment

#### Scenario: Unsupported platform is selected
- **WHEN** the host is any other operating-system and architecture pair
- **THEN** every local workflow rejects it consistently

## ADDED Requirements

### Requirement: Host environment preparation precedes workflow transitions
The system SHALL complete host prerequisite approval and preparation before recording a deploy or start operation as in progress.

#### Scenario: Host preparation is declined
- **WHEN** a required host change is not approved
- **THEN** the command fails without changing the deployment's prior workflow state

#### Scenario: Host preparation fails
- **WHEN** a host prerequisite command fails
- **THEN** the command returns that failure without changing the deployment's prior workflow state

### Requirement: Host container lifecycle remains platform-neutral
The system SHALL use the same direct host container lifecycle when platform differences are limited to environment preparation.

#### Scenario: Linux and Windows host runtimes start Nano
- **WHEN** either supported host platform completes environment preparation
- **THEN** the runtime applies the same deployment-owned persistence, port, SLC, recovery, diagnostic, endpoint, and health policies
