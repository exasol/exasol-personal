# Manage sidecars

Sidecars run alongside the database on a local deployment's host.
The built-in catalog
provides templates; each deployment owns its saved definitions. Use the
catalog names reported by your launcher:

```bash
exasol sidecar list
exasol sidecar enable <name> [--no-wait]
exasol sidecar status [name]
exasol sidecar disable <name> [--remove]
```

Each command accepts `--deployment <name>` or `--deployment-dir <path>` and
`--json`. `list` also answers without a deployment, reporting the catalog with
the architecture support of the machine you run it on. Against a deployment
that cannot run sidecars, every entry reports `supported: false` and says why.
`status` rejects a name that is neither saved, in the catalog, nor associated
with an unresolved operation failure. JSON status includes `name`, `enabled`,
`running`, `state`, `exitCode`, `health`, `restartRequired`, `endpoints`, and
`healthError` and `error` when available. Each endpoint carries `ip` and
`port`, and a `url` when its port declares an application protocol. `state`
describes the container process; `health` describes whether the service inside
it is serving, as described in [Readiness and health](#readiness-and-health).
Without a name, `status` reports every saved sidecar, enabled or disabled, and
its JSON output is an array of those same entries. `exasol info` also lists
every saved sidecar with its enablement and endpoints, read from the saved
definitions without inspecting containers; its JSON output carries them as a
`sidecars` array of `name`, `enabled`, and `endpoints`.

Sidecars use host Podman on Linux, Podman machine on Windows, and the local VM
on macOS. macOS publication requires a runner with live-forwarding support.

## Enablement and lifecycle

Disabling a sidecar stops it and keeps its saved definition with
`enabled: false`, so enabling it again restores your edits, such as a changed
host port. To return a catalog sidecar to its template, remove the definition:

```bash
exasol sidecar disable <name> --remove
exasol sidecar enable <name>
```

Enable and both forms of disable are idempotent. Repeating a disable reports
that there is nothing to remove (`nothingToRemove: true` in JSON).
Enable on an initialized or stopped
deployment records intent for the next deploy or start. Enable on a running
deployment starts the selected sidecar on each host. Some existing local
deployments report `restartRequired: true`; run `exasol stop` followed by
`exasol start` to restart the deployment and start the enabled sidecars.

`exasol start` reconciles saved definitions even when the database is already
running. It keeps unchanged running sidecars, recreates changed or missing
containers, and removes containers whose definitions were deleted or disabled.
Services handle their own connection retries while the database starts.

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
sidecars:
  example:
    enabled: true
    container:
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

The key under `sidecars` names the sidecar and its container. A `name` inside
`container` is accepted when it repeats that key, and reported as an error
when it differs.

`enabled` beside `container` records whether the sidecar runs. Enable writes
`enabled: true` and disable writes `enabled: false`. An entry without the field
is enabled. Set it by hand and run `exasol start` to apply the change.

Supported fields follow the Kubernetes Container shape:

| Field | Supported values |
| --- | --- |
| `name` | The sidecar's key; a DNS label, with `database` reserved |
| `image` | Concrete image reference |
| `imagePullPolicy` | `Always`, `IfNotPresent`, `Never` |
| `command`, `args` | Entrypoint and argument arrays |
| `workingDir` | Container working directory |
| `env` | `name`, literal `value`, or `valueFrom.secretKeyRef` |
| `ports` | `name`, `containerPort`, `hostPort`, `hostIP`, `protocol: TCP` |
| `restartPolicy` | `Always`, `OnFailure`, `Never` |
| `readinessProbe` | See [Readiness and health](#readiness-and-health) |

The default restart policy is `OnFailure`. Explicit stop and disable take
precedence over process restart. Individual Container restart policies require
compatible Kubernetes feature support if these definitions are later used
with Kubernetes.

An untagged image or `latest` tag defaults to `Always`; other tags and digests
default to `IfNotPresent`. Omitted command and arguments retain image defaults.
Unsupported fields and invalid values produce an error with their location.

## Readiness and health

A container's `readinessProbe` tells the launcher how to check that its service
works. The probe declares one handler:

| Handler | Succeeds when |
| --- | --- |
| `exec.command` | The command, run inside the container, exits with status 0 |
| `httpGet` with `port`, `path`, `scheme` | `GET` answers with a status from 200 through 399 |
| `tcpSocket` with `port` | A TCP connection is accepted |

An `httpGet` or `tcpSocket` port is a container port number or port name with
a `hostPort`, because the launcher connects to its published loopback address.
`path` defaults to `/` and `scheme` to `HTTP`; `HTTPS` skips certificate
verification. An `exec` command can check any port from inside the container.

```yaml
      readinessProbe:
        httpGet:
          port: http
          path: /health
        periodSeconds: 10
        failureThreshold: 6
```

The launcher runs the probe whenever status or enable inspects a running
sidecar. `timeoutSeconds` (default 1) limits each attempt. Status reports the
result as `health`:

- `starting` until `initialDelaySeconds` (default 0) has passed since the
  container started, and while the probe fails within its startup window of
  `initialDelaySeconds + periodSeconds × failureThreshold` (`periodSeconds`
  defaults to 10 and `failureThreshold` to 3).
- `serving` when the probe succeeds.
- `failing` when the probe fails after its startup window.

`healthError` reports the cause of the latest probe failure. `health` is empty
for a stopped sidecar or one without a probe. A failing probe leaves the
container running; `restartPolicy` governs process exit. Editing the probe
applies on the next status and keeps the container.

`exasol sidecar enable` waits while a sidecar is `starting`, up to its probe's
startup window, then reports the result. A sidecar that is still starting or
failing gets a call to action to check `exasol sidecar status <name>`. Use
`--no-wait` to return as soon as the container runs. The MCP catalog entry
declares a readiness probe on its `/health` path.

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

### Connection URLs

A port that speaks an application protocol declares it in an `endpoints` list
beside the sidecar's `container`, naming one of that container's ports:

```yaml
version: 1
sidecars:
  mcp:
    container:
      image: docker.io/exasol/mcp-server:2.2.0
      ports:
        - name: mcp
          containerPort: 4896
          hostPort: 4896
    endpoints:
      - port: mcp
        scheme: http
        path: /mcp
```

Status then reports the complete URL, built from the address the port is
actually published on. An edited scheme or path takes effect on the next
`status`, without restarting the sidecar. A port with no declaration reports
only its host and port.

## MCP server

The catalog includes the official Exasol MCP server for local deployments. Manage it with
`exasol mcp`, as described in [Connect an AI agent](ai-agents.md).
