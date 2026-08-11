## MODIFIED Requirements

### Requirement: Local runtime implementation is isolated
The system SHALL keep macOS VM mechanics and Linux host mechanics outside the deployment workflow package.

#### Scenario: Local runner lifecycle is delegated
- **WHEN** the local backend deploys, starts, stops, destroys, reports status, checks health, executes a command, maps an artifact path, or opens a shell
- **THEN** it delegates platform-specific behavior to the selected local runtime component

#### Scenario: Deployment workflow remains the artifact owner
- **WHEN** the selected local runtime reports endpoint state after deploy or start
- **THEN** the local backend maps that state into launcher deployment artifacts and workflow behavior

### Requirement: Local deployment metadata is endpoint-based
The system SHALL write new local deployment metadata using application connection endpoints rather than node or transport metadata.

#### Scenario: Local deployment artifacts omit nodes
- **WHEN** the launcher writes `deployment.json` for a new local deployment
- **THEN** the file omits the top-level `nodes` field

#### Scenario: Local connection metadata contains required endpoints
- **WHEN** the launcher writes `deployment.json` for a running local deployment
- **THEN** `connection` contains loopback SQL endpoint metadata, Admin UI metadata when available, and shell support metadata without an SSH endpoint

#### Scenario: Cloud deployment artifacts preserve nodes
- **WHEN** a tofu-backed cloud deployment writes `deployment.json`
- **THEN** node metadata remains present and unchanged for cloud-specific workflows

### Requirement: Local shell access does not require node metadata
The system SHALL support local shell commands by delegating to the selected runtime without reading SSH details from deployment metadata or runner state.

#### Scenario: Local host shell uses local connection metadata
- **WHEN** a macOS local deployment is running and the user runs `exasol shell host`
- **THEN** the runtime opens an interactive VM shell through its command-execution contract

#### Scenario: Local container shell uses local connection metadata
- **WHEN** a macOS local deployment is running and the user runs `exasol shell container`
- **THEN** the runtime opens its container-oriented shell against the deployment's Nano rootfs and namespaces through its command-execution contract

#### Scenario: Runtime does not support a shell
- **WHEN** the selected local runtime cannot provide the requested shell
- **THEN** the command returns that runtime's explicit unsupported error

### Requirement: Launcher startup state is translated once
The system SHALL translate launcher version-check state, installed SLC state, service ports, initialization parameters, deployment paths, and runtime artifact paths into runtime-neutral installation settings through shared policy.

#### Scenario: Installed SLC state is unavailable
- **WHEN** the caller does not know the deployment's SLC state
- **THEN** the installation receives a nil SLC list

#### Scenario: Deployment has no installed SLCs
- **WHEN** the caller authoritatively knows that no SLCs are configured
- **THEN** the installation receives a non-nil empty SLC list

#### Scenario: Installation receives a materialized artifact
- **WHEN** a runtime materializes an image or custom package for installation
- **THEN** installation receives paired host and runtime paths without inferring the mapping
