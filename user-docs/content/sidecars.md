# Manage sidecars

Sidecars run alongside the database on a deployment's hosts. The built-in
catalog provides templates; each deployment owns its enabled definitions. Use
the catalog names reported by your launcher:

```bash
exasol sidecar list
exasol sidecar enable <name>
exasol sidecar status <name>
exasol sidecar disable <name>
```

Each command accepts `--deployment <name>` or `--deployment-dir <path>` and
`--json`. JSON status includes `name`, `enabled`, and a `hosts` array. Each
host has its own `name`, `running`, `state`, `exitCode`, `restartRequired`,
`endpoints`, and an `error` when available. A deployment without sidecar hosts
reports an empty `hosts` array.

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
catalog template for your service.

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

A `hostPort` must name a loopback `hostIP`, which defaults to `127.0.0.1`.

## Database connection values

The launcher resolves the `exasol-database` source when it creates a container:

| Key | Runtime value |
| --- | --- |
| `host` | The deployment's database host name |
| `port` | The deployment's SQL port |
| `username` | Saved database connection username |
| `password` | Saved database password |

Environment names are selected by the template. Saved definitions retain
references; resolved credentials are supplied at runtime. An unavailable
required reference fails startup. `optional: true` on `secretKeyRef` omits an
unavailable variable.

Command and argument strings expand `$(NAME)` from the container environment.
Literal environment values can refer to earlier entries. `$$` escapes a dollar
sign, and unresolved references remain literal. Expanding a password into
application arguments or output exposes it to that application; choose those
settings with care. Launcher diagnostics redact injected values.
