# Manage sidecars

Sidecars run alongside the database on the local host or every cloud node.
The built-in catalog
provides templates; each deployment owns its enabled definitions. Use the
catalog names reported by your launcher:

```bash
exasol sidecar list
exasol sidecar enable <name>
exasol sidecar status <name>
exasol sidecar disable <name>
```

Each command accepts `--deployment <name>` or `--deployment-dir <path>` and
`--json`. JSON status includes `name`, `enabled`, and a `hosts` array. Each
host has its own `name`, `running`, `state`, `exitCode`, `restartRequired`,
`endpoints`, and an `error` when available. The local host is named `local`;
cloud hosts use their node names. Before provisioning, `hosts` is empty.
Status describes the container process, rather than application readiness.

Sidecars use host Podman on Linux, Podman machine on Windows, and the local VM
on macOS. macOS publication requires a runner with live-forwarding support.
Cloud deployments install each sidecar as a service of the node's database
service, so the node starts, stops, and restarts it together with the
database.

## Enablement and lifecycle

Enable and disable are idempotent. Enable on an initialized or stopped
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
It also retries cleanup when the name is already absent from `sidecars.yaml`.
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
precedence over process restart. On cloud nodes the policy is applied by the
node's service manager, which also stops the sidecar when the database service
stops. Individual Container restart policies require
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
| `port` | Local internal SQL port, or the cloud node's SQL port |
| `username` | Saved database connection username |
| `password` | Saved database password |

Environment names are selected by the template. Saved definitions retain
references; resolved credentials are supplied at runtime. An unavailable
required reference fails startup. `optional: true` on `secretKeyRef` omits an
unavailable variable.

Removing a password reference from a saved definition takes effect at the
next reconciliation.

Command and argument strings expand `$(NAME)` from the container environment.
Literal environment values can refer to earlier entries. `$$` escapes a dollar
sign, and unresolved references remain literal. Expanding a password into
application arguments or output exposes it to that application; choose those
settings with care. Launcher diagnostics redact injected values.

## Networking

Local services share the
[deployment bridge](local-deployment.md#deployment-network).
The database uses the DNS name `database`; each sidecar uses its saved name.
These names resolve inside that deployment's network.

`containerPort` alone declares an internal service port. A positive `hostPort`
publishes it on the requested loopback `hostIP`, defaulting to `127.0.0.1`.
Status reports the effective IP and port. If a host port is occupied, edit the
saved mapping and retry start.

Cloud sidecars share their node's host network. Each container receives a
`database` hosts-file entry pointing to the node's private database address.
A positive `hostPort` must equal `containerPort`. Configure the service itself
to listen on the intended address and port: host networking performs no port
translation, and `hostIP` does not restrict the application's listener.
Omitting `hostPort` does not isolate the service from the node's network.
Status reports declared endpoints from the applied container definition in
that node's network context, including loopback addresses for node-local
access. Sidecar DNS aliases are provided by the local bridge only.

A positive `hostPort` also opens that port in the cloud provider's firewall
for the deployment's allowed address range, alongside the database ports.
Enable and disable apply the change to the provider configuration, and the
open ports follow enablement across stop and start. Declare a `hostPort` only
for services you intend to reach from outside the node, and keep the service's
own listening address in mind: a service bound to loopback stays node-local
even when its port is admitted.
