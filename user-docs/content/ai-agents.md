# Connect an AI agent

A local deployment can serve the official
[Exasol MCP server](https://github.com/exasol/mcp-server), which lets AI agents and other
[MCP](https://modelcontextprotocol.io) clients explore schemas and run read-oriented queries
against your database. The server runs as a [sidecar](sidecars.md) next to the database.

## Enable the MCP server

```bash
exasol mcp enable
```

On a running deployment, the command starts the server and waits until it serves. On a stopped
deployment, the server starts with the next `exasol start`. `exasol mcp status` reports its state
and endpoint, `http://127.0.0.1:4896/mcp` by default, and `exasol mcp disable` stops it. These
commands accept the same options as the matching `exasol sidecar` commands for the sidecar named
`mcp`. `exasol info` lists the endpoint of every enabled sidecar.

If the default host port is occupied, change the mapping in `sidecars.yaml` as described in
[Networking](sidecars.md#networking).

## Connect a client

List the clients the launcher has setup instructions for:

```bash
exasol mcp instructions
```

Show the setup instructions for one of them, for example Claude Code:

```bash
exasol mcp instructions claude-code
```

```text
Run this command to register the MCP server with Claude Code:

  claude mcp add --transport http --scope user exasol-personal http://127.0.0.1:4896/mcp
```

The launcher prints the configuration and leaves your client's settings unchanged. Clients that
offer a registration command get that command; Cursor, Continue, and Claude Desktop get the entry
to add to their configuration file. The printed configuration uses the endpoint saved for your
deployment, so it follows a changed host port and needs an initialized deployment. With `--json`,
the output carries the client, the endpoint URL, and either `command` or `file` and `content`.

Any other client that supports Streamable HTTP connects with the endpoint URL. Then ask the client
to run a query, for example:

> Use `execute_exasol_query` to run
> `SELECT USER_NAME FROM EXA_ALL_USERS WHERE USER_NAME = CURRENT_USER`.

The client returns the current database user (`SYS` for a default local deployment).

## Verify the connection

To verify the server without an agent, use the official
[MCP Inspector](https://github.com/modelcontextprotocol/inspector). Install Node.js, set the
endpoint, and run the following commands:

```bash
MCP_URL=http://127.0.0.1:4896/mcp
npx @modelcontextprotocol/inspector --cli --transport http --server-url "$MCP_URL" --method initialize
npx @modelcontextprotocol/inspector --cli --transport http --server-url "$MCP_URL" --method tools/list
npx @modelcontextprotocol/inspector --cli --transport http --server-url "$MCP_URL" --method tools/call --tool-name execute_exasol_query --tool-args-json '{"query":"SELECT USER_NAME FROM EXA_ALL_USERS WHERE USER_NAME = CURRENT_USER"}'
```

The first command verifies the MCP connection, the second lists the available tools, and the
third reads an existing database system view. A successful result contains the `USER_NAME` column
and the current database user.

## Security

The endpoint listens on the loopback address, has no authentication, and uses the deployment's
saved database credentials. Only connect trusted local MCP clients. The server offers
read-oriented query tools and disables write-query, BucketFS, and preprocessor tools. Queries that
do not specify `row_limit` return at most 1000 rows by default; clients can request a different
limit. These controls limit the exposed MCP tools; they do not change the database privileges of
the saved user.
