# Manage sidecars

Sidecars run alongside the database on a local deployment's host.
The built-in catalog
provides templates; each deployment owns its enabled definitions. Use the
catalog names reported by your launcher:

```bash
exasol sidecar list
exasol sidecar enable <name>
exasol sidecar status [name]
exasol sidecar disable <name>
```

Each command accepts `--deployment <name>` or `--deployment-dir <path>` and
`--json`. `list` also answers without a deployment, reporting the catalog with
the architecture support of the machine you run it on. Against a deployment
that cannot run sidecars, every entry reports `supported: false` and says why.
`status` rejects a name that is neither saved, in the catalog, nor associated
with an unresolved operation failure. JSON status includes `name`, `enabled`,
`running`, `state`, `exitCode`, `restartRequired`, `endpoints`, and an `error`
when available.
Status describes the container process, rather than application readiness.
Without a name, `status` reports every enabled sidecar, and its JSON output is
an array of those same entries.

Sidecars use host Podman on Linux, Podman machine on Windows, and the local VM
on macOS. macOS publication requires a runner with live-forwarding support.

## Enablement and lifecycle

Enable and disable are idempotent. Disabling a sidecar that is already absent
reports that there is nothing to remove (`nothingToRemove: true` in JSON).
Enable on an initialized or stopped
deployment records intent for the next deploy or start. Enable on a running
deployment starts the selected sidecar on each host. Some existing local
deployments report `restartRequired: true`; run `exasol stop` followed by
`exasol start` to restart the deployment and start the enabled sidecars.

`exasol start` reconciles saved definitions even when the database is already
running. It keeps unchanged running sidecars, recreates changed or missing
containers, and removes containers whose definitions were deleted. Services
handle their own connection retries while the database starts.

`exasol stop` removes disposable sidecar containers while retaining definitions.
`exasol destroy` removes owned containers and the deployment network. Failures
are reported separately from database state. Fix the reported problem and
retry enable or start. A failed disable can be retried to finish cleanup.
Failures identify the affected host and sidecar. Other hosts are still
reconciled, and retries use the same deployment-wide saved definition.

`exasol sidecar disable <name>` accepts user-added or renamed saved entries.
It also retries cleanup when the name is absent from `sidecars.yaml` but an
unresolved cleanup failure remains recorded.
`exasol start` retries removal of owned containers absent from desired state.
Cleanup requires a reachable host and leaves the database deployment intact.

Status queries current runtime state and endpoints. Its per-host
`reconciliation` is `complete` when the running container matches the saved
definition and published mappings, or when required cleanup is complete.
It is `pending` when changes remain and `unknown` when inspection cannot
verify the outcome. A stopped database requires sidecar cleanup even if its
container host remains available.

The launcher stores unresolved operation failures in `sidecars-state.json`.
Status clears a recorded failure only after verifying the desired outcome.
`lastOperationError` reports any retained failure; `error` reports a current
inspection failure. Status updates these records while leaving containers,
forwards, and saved definitions untouched. Use enable, disable, or start to
apply pending changes. Successful operations also clear their recorded error.

## Edit deployment configuration

The launcher manages `<deployment-dir>/sidecars.yaml` and accepts user edits.
The saved definitions are authoritative. Re-enabling a name preserves those
edits, and catalog upgrades affect newly enabled entries.

```yaml
version: 1
containers:
  - name: example
    image: registry.example.org/service:1
    imagePullPolicy: IfNotPresent
    restartPolicy: OnFailure
    ports:
      - containerPort: 8080
        hostPort: 18080
        hostIP: 127.0.0.1
    env:
      - name: EXASOL_DB_HOST
        valueFrom:
          secretKeyRef:
            name: exasol-database
            key: host
      - name: EXASOL_DB_PASSWORD
        valueFrom:
          secretKeyRef:
            name: exasol-database
            key: password
```

The example illustrates the schema. Use the concrete image supplied by a
catalog template for your service, then apply edits with `exasol start`.

Supported fields follow the Kubernetes Container shape:

| Field | Supported values |
| --- | --- |
| `name` | Unique DNS label; `database` is reserved |
| `image` | Concrete image reference |
| `imagePullPolicy` | `Always`, `IfNotPresent`, `Never` |
| `command`, `args` | Entrypoint and argument arrays |
| `workingDir` | Container working directory |
| `env` | `name`, literal `value`, or `valueFrom.secretKeyRef` |
| `ports` | `name`, `containerPort`, `hostPort`, `hostIP`, `protocol: TCP` |
| `restartPolicy` | `Always`, `OnFailure`, `Never` |

The default restart policy is `OnFailure`. Explicit stop and disable take
precedence over process restart. Individual Container restart policies require
compatible Kubernetes feature support if these definitions are later used
with Kubernetes.

An untagged image or `latest` tag defaults to `Always`; other tags and digests
default to `IfNotPresent`. Omitted command and arguments retain image defaults.
Unsupported fields and invalid values produce an error with their location.

## Database connection values

The launcher resolves the `exasol-database` source at container creation:

| Key | Runtime value |
| --- | --- |
| `host` | `database` |
| `port` | The deployment's internal SQL port |
| `username` | Saved database connection username |
| `password` | Saved database password |

Environment names are selected by the template. Saved definitions retain
references; resolved credentials are supplied at runtime. An unavailable
required reference fails startup. `optional: true` on `secretKeyRef` omits an
unavailable variable.

Removing a password reference from a saved definition takes effect at the
next reconciliation. Changing a database connection value, such as the
password, replaces the running services that use it on the next
reconciliation.

Command and argument strings expand `$(NAME)` from the container environment.
Literal environment values can refer to earlier entries. `$$` escapes a dollar
sign, and unresolved references remain literal. Expanding a password into
application arguments or output exposes it to that application; choose those
settings with care. Launcher diagnostics withhold injected passwords and keep
other values legible.

## Networking

Local services share the
[deployment bridge](local-deployment.md#deployment-network).
The database uses the DNS name `database`; each sidecar uses its saved name.
These names resolve inside that deployment's network.

`containerPort` alone declares an internal service port. A positive `hostPort`
publishes it on the requested loopback `hostIP`, defaulting to `127.0.0.1`.
Status reports the effective IP and port. A sidecar that fails to start names
the cause and points at `exasol sidecar status <name>`, which reports the full
diagnosis.

## MCP server

For a local deployment, enable the official Exasol MCP server with:

```bash
exasol sidecar enable mcp
exasol sidecar status mcp
```

The default MCP endpoint is `http://127.0.0.1:4896/mcp`. The `Endpoint` line
that `exasol sidecar status mcp` prints shows only the host address and port,
for example `127.0.0.1:4896`; MCP clients need the full URL, so prefix it with
`http://` and append `/mcp`. Use the status output to obtain the effective
address after changing the saved port mapping. If the default host port is
occupied, change the mapping as described in [Networking](#networking).

To verify the complete connection with the official
[MCP Inspector](https://github.com/modelcontextprotocol/inspector), install
Node.js, set the endpoint (adjust the port if you changed the mapping), and run
the following commands:

```bash
MCP_URL=http://127.0.0.1:4896/mcp
npx @modelcontextprotocol/inspector --cli --transport http --server-url "$MCP_URL" --method initialize
npx @modelcontextprotocol/inspector --cli --transport http --server-url "$MCP_URL" --method tools/list
npx @modelcontextprotocol/inspector --cli --transport http --server-url "$MCP_URL" --method tools/call --tool-name execute_exasol_query --tool-args-json '{"query":"SELECT USER_NAME FROM EXA_ALL_USERS WHERE USER_NAME = CURRENT_USER"}'
```

The first command verifies the MCP connection, the second lists the available
tools, and the third reads an existing database system view. A successful
result contains the `USER_NAME` column and the current database user (`SYS` for
a default local deployment).

To use the server from your own MCP client, register the same URL in that
client as a Streamable HTTP server; the exact steps depend on the client. Then
ask the client to run a query, for example:

> Use `execute_exasol_query` to run
> `SELECT USER_NAME FROM EXA_ALL_USERS WHERE USER_NAME = CURRENT_USER`.

The client returns the current database user (`SYS` for a default local
deployment).

The endpoint has no authentication and uses the deployment's saved database
credentials. Only connect trusted local MCP clients. The catalog enables
read-oriented query tools and disables write-query, BucketFS, and preprocessor
tools. These controls limit the exposed MCP tools; they do not change the
database privileges of the saved user.
