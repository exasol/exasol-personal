## ADDED Requirements

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
