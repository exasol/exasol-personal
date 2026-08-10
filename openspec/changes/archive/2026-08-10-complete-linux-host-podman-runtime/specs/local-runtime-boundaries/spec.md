## MODIFIED Requirements

### Requirement: Local runtime implementation is isolated
The system SHALL keep macOS VM runner mechanics and Linux host Podman mechanics outside the deployment workflow package.

#### Scenario: Local runner lifecycle is delegated
- **WHEN** the local backend deploys, starts, stops, destroys, reports status, or checks health
- **THEN** it delegates platform-specific command execution, runtime paths, endpoint state, and installation behavior to the selected local runtime component

#### Scenario: Deployment workflow remains the artifact owner
- **WHEN** the selected local runtime reports endpoint state after deploy or start
- **THEN** the local backend maps that state into launcher deployment artifacts and workflow behavior

## ADDED Requirements

### Requirement: Local runtime selection is centralized
The system SHALL use one platform-selection policy for backend creation, status reconciliation, SLC restart decisions, connection diagnostics, and local diagnostics.

#### Scenario: Supported platform is selected
- **WHEN** the host is macOS Apple Silicon, Linux AMD64, or Linux ARM64
- **THEN** every local workflow selects the same corresponding VM or host runtime

#### Scenario: Unsupported platform is selected
- **WHEN** the host is any other operating-system and architecture pair
- **THEN** every local workflow rejects it consistently

### Requirement: Launcher startup state is translated once
The system SHALL translate launcher version-check state, installed SLC state, ports, initialization parameters, and deployment paths into runtime-neutral installation settings through shared policy.

#### Scenario: Installed SLC state is unavailable
- **WHEN** the caller does not know the deployment's SLC state
- **THEN** the installation receives a nil SLC list

#### Scenario: Deployment has no installed SLCs
- **WHEN** the caller authoritatively knows that no SLCs are configured
- **THEN** the installation receives a non-nil empty SLC list
