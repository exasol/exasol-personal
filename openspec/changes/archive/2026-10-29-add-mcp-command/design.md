## Context

See proposal.md for motivation.

`install` and `start` finish by printing connection instructions built from
the deployment info report, with one `===` section per way to reach the
database. Guidance goes to standard
error for text output and is suppressed under `--json`, where agents read
structured fields instead.

Sidecars run only on local deployments. The MCP server is the catalog entry
named `mcp`. Enabling it saves a definition in the deployment's
`sidecars.yaml`, whose host port and endpoint declaration determine the URL.
The catalog is embedded in the launcher and declares the architectures each
entry supports.

## Goals / Non-Goals

**Goals:**

- Top-level help shows users and agents that an MCP server is available.
- One command family covers enabling and checking the MCP server and setting up
  its clients.
- An agent finds every sidecar's state and URL in `exasol info --json`.
- Users of the listed clients can connect by copying one printed command or
  snippet.

**Non-Goals:**

- Writing client configuration files or running client commands.
- Enabling the MCP server by default.
- Reporting sidecar health in info; `exasol mcp status` and
  `exasol sidecar status` remain the place for it.

## Decisions

**`exasol mcp` is an alias family over the sidecar commands.** Its `enable`,
`disable`, and `status` subcommands call the same implementations as
`exasol sidecar <subcommand> mcp`, so output, JSON, errors, and calls to action
stay identical and the sidecar specification remains the single definition of
their behavior. `exasol sidecar` keeps working for every sidecar, MCP included.

**The command's presence follows the catalog.** The launcher registers `mcp`
when its embedded catalog has an `mcp` entry that supports the host
architecture, the same condition `exasol sidecar list` reports as supported.
Both are known when the launcher starts, so help output is stable. Deployment
backend support is not known until a deployment is resolved, so against a
deployment that cannot run sidecars the subcommands fail the way the sidecar
commands do. A saved `mcp` definition on a host without a supported catalog
entry stays reachable through `exasol sidecar`.

**`instructions` prints and does not write.** The name says what the command
does: it neither registers the client nor opens a connection, and it succeeds
while the server is stopped, unlike `exasol connect`, which opens a SQL
session. Client configuration formats change between client releases, users
often keep configuration under version control, and the starter kit already
writes client configuration. Printing a
command or snippet keeps the launcher free of client-specific file editing,
makes the output safe for an agent to request, and lets the user choose the
client's scope. Clients with a registration command get that command; the
others get the configuration snippet and the file it belongs in. Claude
Desktop reads only stdio servers from its configuration file, so its snippet
runs a stdio-to-HTTP bridge through `npx`.

**`instructions` uses the saved definition's URL and requires an enabled
server.**
The URL comes from the saved `mcp` definition, the same way info builds it, so
it is correct before the deployment starts and follows an edited host port.
Printing configuration for a server that is not enabled would register an
endpoint that does not answer, so `instructions <client>` fails with a call to
action to run `exasol mcp enable`. Without an initialized deployment, it fails
with an explanation that the instructions use the deployment's endpoint and
calls to action to install a local deployment or select an existing one.
Listing clients works in any state.

**Info lists saved sidecars, not the catalog.** Saved definitions are what the
deployment runs, they include user-added sidecars, and they carry the URLs an
agent needs. Catalog entries that are not enabled stay in
`exasol sidecar list`; the enable suggestion covers the MCP server. Each entry
reports `name`, `enabled`, and `endpoints` with the field names status uses,
built from the saved definition without inspecting containers, so info stays
fast in every deployment state.

**The MCP server gets its own connection instructions section.** The MCP
endpoint is another way to reach the database, so it sits beside the SQL
client, CLI, and shell sections that `install` and `start` print, showing the
endpoint and `exasol mcp instructions` once enabled and `exasol mcp enable`
before. Both appear only for a running deployment, the state `install` and
`start` end in, so the suggestion does not compete with recovery guidance in
other states.

**Info groups running-state guidance by goal.** Opening a SQL session is the
main next step and stopping the deployment the last, so the optional MCP step
sits between them instead of competing with them. Once the server is enabled,
the step is `exasol mcp instructions` without a client, which lists the
clients, so every printed command can be pasted as is.

**Printed commands select non-default deployments only, and the launcher
reports a deployment taken from the current directory.** Printing
`-d default` in every command would lengthen the guidance for the common
single-deployment setup. A command without a selection follows the current
directory when it is a deployment directory, so the launcher reports that
deployment the way it already reports the default and named ones, and a
pasted command shows which deployment it acted on.

**`llms.txt` is a static file.** The documentation site copies non-Markdown
files unchanged, so a hand-written `llms.txt` needs no build plugin. It links
only to the AI agent page and the documentation home, which keeps it from
duplicating the navigation.

## Risks / Trade-offs

- Client command syntax drifts. Each client's instructions are verified
  against its current documentation during implementation, and the printed
  output names the client configuration it targets so a user can recognize a
  mismatch.
- A user who does not want MCP sees the suggestion on every start. It is one
  line of guidance on standard error and disappears once the server is
  enabled.
- An enabled sidecar whose container failed still shows its URL in info. The
  status commands report its health.
