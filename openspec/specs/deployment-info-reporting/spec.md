# deployment-info-reporting Specification

## Purpose
TBD - created by archiving change improve-deployment-info-guidance. Update Purpose after archive.
## Requirements
### Requirement: Info SHALL report uninitialized deployment directories

`exasol info` SHALL succeed when the resolved deployment directory has no initialized deployment yet, and SHALL report that state to the user.

#### Scenario: Text output before initialization

- **WHEN** a user runs `exasol info` for a resolved deployment directory with no initialized deployment
- **THEN** the command succeeds
- **AND** the output identifies the resolved deployment directory
- **AND** the output reports the deployment state as `not_initialized`
- **AND** the output tells the user how to discover presets and create a deployment

#### Scenario: JSON output before initialization

- **WHEN** a user runs `exasol info --json` for a resolved deployment directory with no initialized deployment
- **THEN** the command succeeds
- **AND** stdout contains valid JSON
- **AND** the JSON contains the resolved deployment directory
- **AND** the JSON reports `deploymentState` as `not_initialized`

### Requirement: Info text output SHALL guide users by deployment state

Human-readable `exasol info` output SHALL include concise next-step guidance appropriate to the current deployment state.

#### Scenario: Initialized deployment

- **WHEN** a user runs `exasol info` for an initialized deployment that has not been deployed yet
- **THEN** the output reports the initialized state
- **AND** the output points the user toward deploying the database

#### Scenario: Running deployment

- **WHEN** a user runs `exasol info` for a running deployment
- **THEN** the output reports the running state
- **AND** the output points the user toward connecting to or stopping the deployment

#### Scenario: Stopped deployment

- **WHEN** a user runs `exasol info` for a stopped deployment
- **THEN** the output reports the stopped state
- **AND** the output points the user toward starting or destroying the deployment

#### Scenario: Deployment operation is not complete

- **WHEN** a user runs `exasol info` while a deployment operation is in progress, interrupted, or failed
- **THEN** the output reports the current state
- **AND** the output gives a state-appropriate next step

### Requirement: Info JSON output SHALL be machine-readable deployment information

`exasol info --json` SHALL write only valid JSON to stdout and SHALL provide a structured deployment overview suitable for scripts and agents.

#### Scenario: JSON stdout is parseable

- **WHEN** a user runs `exasol info --json`
- **THEN** stdout contains valid JSON
- **AND** stdout does not contain banners, prompts, or explanatory terminal prose outside JSON

#### Scenario: Initialized deployment metadata is available

- **WHEN** a user runs `exasol info --json` for an initialized deployment
- **THEN** the JSON includes the deployment state
- **AND** the JSON includes the resolved deployment directory
- **AND** the JSON includes the deployment identifier when one is available
- **AND** the JSON includes selected preset information when preset identity is available

#### Scenario: Running deployment connection details are available

- **WHEN** a user runs `exasol info --json` for a running deployment with connection details
- **THEN** the JSON includes connection-relevant fields such as backend, host, database port, username, TLS guidance, Admin UI details when available, and shell access details when available

#### Scenario: Connection details are not stable for the current state

- **WHEN** a user runs `exasol info --json` for a deployment state where connection details are absent or not stable
- **THEN** the JSON still reports the deployment state and resolved deployment directory
- **AND** connection details are omitted

### Requirement: Info SHALL preserve deployment lifecycle behavior

`exasol info` SHALL report deployment information without creating, modifying, starting, stopping, destroying, or removing deployment resources.

#### Scenario: Information command does not mutate deployment lifecycle

- **WHEN** a user runs `exasol info` or `exasol info --json`
- **THEN** the command reports the current deployment information
- **AND** the command does not change the deployment lifecycle state

### Requirement: Info SHALL report the saved sidecars of deployments that support sidecars

`exasol info` SHALL report each saved sidecar of a deployment that supports sidecars with its name, its enablement, and the endpoints built from its saved published ports and endpoint declarations.

#### Scenario: Text output with saved sidecars

- **WHEN** a user runs `exasol info` for an initialized local deployment with saved sidecars
- **THEN** the output lists each saved sidecar with its name and whether it is enabled
- **AND** the output includes the endpoint URL of each enabled sidecar that declares an endpoint

#### Scenario: JSON output with saved sidecars

- **WHEN** a user runs `exasol info --json` for an initialized local deployment with saved sidecars
- **THEN** the JSON contains a `sidecars` array with one object per saved sidecar in name order
- **AND** each object contains `name`, `enabled`, and `endpoints`
- **AND** each endpoint carries `ip` and `port`, and `url` when its port declares an endpoint

#### Scenario: JSON output without saved sidecars

- **WHEN** a user runs `exasol info --json` for an initialized local deployment without saved sidecars
- **THEN** the JSON contains an empty `sidecars` array

#### Scenario: Edited host port

- **WHEN** a user changes a sidecar's saved host port and runs `exasol info --json`
- **THEN** that sidecar's endpoint uses the edited host port

### Requirement: Info guidance SHALL group running-deployment steps by goal

The next-step guidance of `exasol info` for a running deployment SHALL show the command that opens a SQL session first and the command that stops the deployment last. Between them, the guidance SHALL show `exasol mcp enable`, marked as optional, for a deployment that supports sidecars, runs on a host architecture that the catalog's `mcp` entry supports, and does not have the MCP server enabled, and `exasol mcp instructions` for a deployment with the MCP server enabled.

#### Scenario: Info guidance for a running deployment without MCP

- **WHEN** a user runs `exasol info` for a running local deployment without the MCP server enabled
- **THEN** the next-step guidance shows `exasol connect` first and `exasol stop` last
- **AND** between them, the guidance marks the MCP server as optional and includes `exasol mcp enable`

#### Scenario: Info guidance for a running deployment with MCP

- **WHEN** a user runs `exasol info` for a running local deployment with the MCP server enabled
- **THEN** the next-step guidance shows `exasol connect` first and `exasol stop` last
- **AND** between them, the guidance includes `exasol mcp instructions` without a client placeholder

### Requirement: Connection instructions SHALL include an MCP server section

The connection instructions that `exasol install` and `exasol start` print for a running deployment that supports sidecars SHALL include a section titled `=== MCP Server for AI Agents ===` when the MCP server is enabled with an endpoint or when the catalog's `mcp` entry supports the host architecture.

#### Scenario: MCP server enabled

- **WHEN** `exasol install local` or `exasol start` completes for a local deployment with the MCP server enabled
- **THEN** the MCP server section shows the endpoint URL and `exasol mcp instructions`

#### Scenario: MCP server not enabled

- **WHEN** `exasol install local` or `exasol start` completes for a local deployment without the MCP server enabled
- **THEN** the MCP server section shows `exasol mcp enable`

