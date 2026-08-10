# local-runtime-boundaries Specification

## Purpose
TBD - created by archiving change refactor-local-runtime-boundaries. Update Purpose after archive.
## Requirements
### Requirement: Local runtime implementation is isolated
The system SHALL keep macOS VM runner mechanics and Linux host Podman mechanics outside the deployment workflow package.

#### Scenario: Local runner lifecycle is delegated
- **WHEN** the local backend deploys, starts, stops, destroys, reports status, or checks health
- **THEN** it delegates platform-specific command execution, runtime paths, endpoint state, and installation behavior to the selected local runtime component

#### Scenario: Deployment workflow remains the artifact owner
- **WHEN** the selected local runtime reports endpoint state after deploy or start
- **THEN** the local backend maps that state into launcher deployment artifacts and workflow behavior

### Requirement: Local deployment metadata is endpoint-based
The system SHALL write new local deployment metadata using connection endpoints rather than node metadata.

#### Scenario: Local deployment artifacts omit nodes
- **WHEN** the launcher writes `deployment.json` for a new local deployment
- **THEN** the file omits the top-level `nodes` field

#### Scenario: Local connection metadata contains required endpoints
- **WHEN** the launcher writes `deployment.json` for a running local deployment
- **THEN** `connection` contains loopback SQL endpoint metadata, Admin UI metadata when available, SSH port metadata, and shell support metadata

#### Scenario: Cloud deployment artifacts preserve nodes
- **WHEN** a tofu-backed cloud deployment writes `deployment.json`
- **THEN** node metadata remains present and unchanged for cloud-specific workflows

### Requirement: Local shell access does not require node metadata
The system SHALL support local shell commands without reading SSH details from `nodes`.

#### Scenario: Local host shell uses local connection metadata
- **WHEN** a local deployment is running and the user runs `exasol shell host`
- **THEN** the launcher opens SSH through the local loopback SSH endpoint and local runtime key without requiring `nodes`

#### Scenario: Local container shell uses local connection metadata
- **WHEN** a local deployment is running and the user runs `exasol shell container`
- **THEN** the launcher opens an interactive shell in the Exasol Local database container through the local loopback SSH endpoint and local runtime key without requiring `nodes`

### Requirement: Existing deployment metadata remains readable
The system SHALL preserve compatibility with existing deployment metadata that still contains node-derived details.

#### Scenario: Existing local metadata contains nodes
- **WHEN** the launcher reads an existing local `deployment.json` that contains `nodes`
- **THEN** connection, status, and shell behavior continue to work where the required connection metadata can be resolved

#### Scenario: Existing cloud metadata contains nodes
- **WHEN** the launcher reads a cloud `deployment.json` that contains `nodes`
- **THEN** node-derived SSH and connection behavior remains unchanged

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

