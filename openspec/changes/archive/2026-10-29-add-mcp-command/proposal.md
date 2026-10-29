## Why

A local deployment can run the official Exasol MCP server as a sidecar, but
nothing outside the sidecar user page mentions it. `install`, `start`, `info`,
and the top-level help never point to it, and once it runs, connecting an MCP
client means finding the endpoint and looking up that client's configuration
format. Users and AI agents exploring the launcher have no way to learn that
an MCP server is one command away, or how to connect to it.

## What Changes

- A top-level `exasol mcp` command appears when the built-in catalog offers
  an MCP server for the host architecture. `exasol mcp enable`, `disable`, and
  `status` behave as `exasol sidecar enable|disable|status mcp`, with the same
  flags.
- `exasol mcp instructions` lists the MCP clients with setup instructions, and
  `exasol mcp instructions <client>` prints the command or configuration that
  registers the MCP server's endpoint with that client: Claude Code, Claude
  Desktop, Codex, Cursor, VS Code (GitHub Copilot), Gemini CLI, OpenCode, and
  Continue. The launcher prints the configuration and leaves applying it to
  the user or agent. Without an initialized deployment, it explains that the
  instructions use the deployment's MCP endpoint and how to create or select
  one.
- `exasol info` reports every saved sidecar of a deployment that supports
  sidecars, with its enablement and endpoint URLs, in text and `--json`.
- The connection instructions that `install` and `start` print gain an
  `=== MCP Server for AI Agents ===` section showing the endpoint and
  `exasol mcp instructions`, or `exasol mcp enable` while the server is not
  enabled. The next-step guidance of `exasol info` for a running deployment
  groups its commands by goal: opening a SQL session first, then the optional
  MCP step, `exasol mcp enable` or `exasol mcp instructions`, and stopping the
  deployment last. Enabling the MCP server suggests `exasol mcp instructions`.
- Commands that act on the deployment in the current working directory report
  it, as they already do for the default and named deployments, so a pasted
  command without a deployment selection shows which deployment it used.
- A new user documentation page, "Connect an AI agent", replaces the MCP
  section of the sidecar page. A static `llms.txt` at the documentation root
  points agents to that page.
- The README tells users that a local deployment can serve an MCP server for
  AI agents and links to the "Connect an AI agent" page.

## Capabilities

### New Capabilities

- `mcp-command`: the `exasol mcp` command, its sidecar subcommands, and client
  connection instructions.

### Modified Capabilities

- `deployment-info-reporting`: info reports saved sidecars and groups its
  running-state guidance by goal, including the MCP server, and connection
  instructions show the MCP server.
- `deployment-directory-resolution`: a deployment selected from the current
  working directory is reported like the default and named deployments.

## Impact

- The CLI gains the `mcp` command, reusing the sidecar subcommand
  implementations, and a client catalog that renders connection instructions
  from the endpoint URL.
- The deployment info report gains a sidecar section built from the saved
  definitions, and the info text, notice, and JSON renderers and the
  connection instructions show it.
- The regenerated CLI reference, a new user page, a navigation entry,
  `llms.txt`, the README, and the changelog.
