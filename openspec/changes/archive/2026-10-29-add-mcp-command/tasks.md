Each implementation task below maps to exactly one commit that passes the full
check suite. Group 7 is validation and produces no commit.

## 1. Sidecars in deployment info

- [x] 1.1 Add a sidecar section to the deployment info report for deployments
  that support sidecars, built from the saved definitions without inspecting
  containers, and render it in info text and as the `sidecars` array in
  `info --json`. Verify `task tests-unit` covers enabled and disabled entries,
  no saved entries, an edited host port, an entry without an endpoint
  declaration, and a deployment that cannot run sidecars. Add launcher tests
  marked `openspec("deployment-info-reporting")` for the text and JSON output
  with and without saved sidecars.

## 2. The mcp command

- [x] 2.1 Add the top-level `mcp` command, registered when the catalog's `mcp`
  entry supports the host architecture, with `enable`, `disable`, and `status`
  subcommands that reuse the sidecar implementations and flags with the name
  `mcp`, and regenerate the CLI reference. Verify `task tests-unit` covers
  registration with and without a supported catalog entry. Add launcher tests
  marked `openspec("mcp-command")` showing the command in top-level help and
  matching `exasol sidecar` output for enable, status, and disable.

## 3. Client connection instructions

- [x] 3.1 Add `exasol mcp instructions` with the client listing, per-client
  instructions rendered from the saved `mcp` definition's URL, JSON output, the
  not-enabled error with its guidance, the missing-deployment error with
  install and selection guidance, and the unknown-client error, and
  regenerate the CLI reference. Verify each client's instructions against its
  current documentation and by registering the endpoint with the clients
  available on the development machine. Verify `task tests-unit` covers the
  rendered instructions for every client, an edited host port, and both
  errors. Add launcher tests marked `openspec("mcp-command")` for the listing,
  one command-based and one file-based client in text and JSON, the
  not-enabled error, and the missing-deployment error.

## 4. Calls to action

- [x] 4.1 Add the `=== MCP Server for AI Agents ===` section to the connection
  instructions, group the running-state next-step guidance of `exasol info`
  by goal with `exasol mcp enable` as an optional step when the deployment
  supports sidecars, the catalog's `mcp` entry supports the host architecture,
  and the MCP server is not enabled, and `exasol mcp instructions` once it is
  enabled, and add `exasol mcp instructions` to the guidance of a serving MCP
  enable. Verify `task tests-unit` covers the section with the MCP server
  enabled, suggested, and unavailable, the guidance with and without the MCP
  server enabled, without a supported catalog entry, and for a deployment that
  cannot run sidecars.

## 5. Documentation

- [x] 5.1 Add the "Connect an AI agent" user page covering `exasol mcp enable`,
  `exasol mcp instructions`, verifying the connection, and the server's security
  properties, replacing the MCP section of the sidecar page with a link to it.
  Document the sidecar section of `exasol info`. Add the page to the
  navigation, add a static `llms.txt` linking to it and the documentation home,
  link the page from the README, and add a changelog entry with example
  invocations. Verify `task docs-check` passes.

## 6. Deployment visibility

- [x] 6.1 Report a deployment selected from the current working directory with
  a notice on standard error, like the default and named deployments. Add a
  launcher test that runs a command from inside a deployment directory and
  checks the notice appears once.

## 7. Validation

- [ ] 7.1 Run `task all` and confirm formatting, linting, unit tests and the
  launcher tests pass on the finished branch. _(Formatting, linting, the
  launcher tests, and the documentation checks pass. 14 unit tests in the
  deploy, local install, local runtime, and runtime artifact packages fail on
  the development machine, with and without the sandbox, and the same tests
  fail on `main` there. Confirm on CI.)_
