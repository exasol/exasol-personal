# mcp-command Specification

## Purpose
Define how users enable and inspect the MCP server of a local deployment and
set up AI agents to use it through the `exasol mcp` command.
## Requirements
### Requirement: The mcp command is available when the catalog offers an MCP server

The launcher SHALL offer a top-level `exasol mcp` command, listed in top-level help, when its catalog has an `mcp` entry that supports the host architecture.

#### Scenario: Top-level help lists the mcp command

- **WHEN** a user runs `exasol --help` on a host architecture that the catalog's `mcp` entry supports
- **THEN** the output lists the `mcp` command with a description naming the MCP server for AI agents

### Requirement: The mcp lifecycle subcommands act on the MCP server sidecar

`exasol mcp enable`, `exasol mcp disable`, and `exasol mcp status` SHALL accept the flags of `exasol sidecar enable`, `exasol sidecar disable`, and `exasol sidecar status` and SHALL produce the same result, output, and exit status as those commands with the sidecar name `mcp`.

#### Scenario: Enable through the mcp command

- **WHEN** a user runs `exasol mcp enable` for a running local deployment
- **THEN** the result matches `exasol sidecar enable mcp`, including its readiness wait and `--no-wait`

#### Scenario: Status through the mcp command

- **WHEN** a user runs `exasol mcp status --json` for a local deployment with the MCP server enabled
- **THEN** stdout contains the same JSON object as `exasol sidecar status mcp --json`

#### Scenario: Disable through the mcp command

- **WHEN** a user runs `exasol mcp disable --remove` for a local deployment with a saved `mcp` definition
- **THEN** the result matches `exasol sidecar disable mcp --remove`

### Requirement: Instructions list the MCP clients with setup instructions

`exasol mcp instructions` without a client SHALL list the identifiers of the clients with setup instructions, with their display names: `claude-code`, `claude-desktop`, `codex`, `cursor`, `vscode`, `gemini`, `opencode`, and `continue`.

#### Scenario: Client listing

- **WHEN** a user runs `exasol mcp instructions`
- **THEN** the output lists each client identifier with its display name

#### Scenario: Client listing as JSON

- **WHEN** a user runs `exasol mcp instructions --json`
- **THEN** stdout contains a JSON array with one object per client with setup instructions, carrying its identifier and display name

### Requirement: Instructions print client configuration for the MCP server endpoint

`exasol mcp instructions <client>` SHALL print, for a deployment with the MCP server enabled, the command or configuration that registers the MCP server's endpoint URL with that client, using the URL built from the saved `mcp` definition.

#### Scenario: Client with a registration command

- **WHEN** a user runs `exasol mcp instructions claude-code` for a local deployment with the MCP server enabled
- **THEN** the output contains a `claude mcp add` command that registers the endpoint URL as a Streamable HTTP server

#### Scenario: Client configured through a file

- **WHEN** a user runs `exasol mcp instructions cursor` for a local deployment with the MCP server enabled
- **THEN** the output names the client's configuration file and contains the configuration entry with the endpoint URL

#### Scenario: Edited host port

- **WHEN** a user changes the MCP server's saved host port and runs `exasol mcp instructions <client>`
- **THEN** the printed configuration uses the edited host port

#### Scenario: Instructions output as JSON

- **WHEN** a user runs `exasol mcp instructions <client> --json` for a local deployment with the MCP server enabled
- **THEN** stdout contains a JSON object with the client identifier, the endpoint URL, and either the command or the configuration file and its content

#### Scenario: MCP server not enabled

- **WHEN** a user runs `exasol mcp instructions <client>` for a local deployment without the MCP server enabled
- **THEN** the command fails with an error stating that the MCP server is not enabled
- **AND** the guidance includes `exasol mcp enable`

#### Scenario: No initialized deployment

- **WHEN** a user runs `exasol mcp instructions <client>` and the selected deployment directory is not initialized
- **THEN** the command fails with an error that names the deployment directory and states that client setup instructions use the selected deployment's MCP endpoint
- **AND** the guidance includes `exasol install local` and `exasol mcp instructions <client> -d NAME`

#### Scenario: Client without setup instructions

- **WHEN** a user runs `exasol mcp instructions` with a client identifier without setup instructions
- **THEN** the command fails with an error that lists the client identifiers with setup instructions
- **AND** the guidance includes `exasol mcp status` for reading the endpoint

### Requirement: Enabling the MCP server suggests client setup instructions

The next-step guidance of `exasol mcp enable` and `exasol sidecar enable mcp` SHALL include `exasol mcp instructions` when the MCP server is serving after the command.

#### Scenario: Guidance after enable

- **WHEN** `exasol mcp enable` completes and the MCP server is serving
- **THEN** the next-step guidance includes `exasol mcp instructions`

