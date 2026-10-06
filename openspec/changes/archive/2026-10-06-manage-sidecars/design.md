## Context

See [the proposal](proposal.md) for motivation and scope.

The SLC catalog already separates embedded templates, pure catalog lookup,
deployment state, and runtime execution. Sidecars follow those boundaries.
The local installation runs Podman through an execution environment shared by
Linux, Windows, and the macOS VM. Deployment operations already use exclusive
locking. The current start path returns early for a ready database.

The macOS runtime supplies named guest-to-host forwards when starting its VM.
Immediate enable and disable also require live forwarding updates. That
capability is a runtime prerequisite, described below.

## Goals / Non-Goals

**Goals:** Keep catalog entries self-contained, saved definitions authoritative,
and container settings compatible with a documented Kubernetes schema subset.
Keep execution and networking behind per-host adapters. Apply one saved
definition to the local host or every cloud node through shared orchestration.

**Non-Goals:** Kubernetes execution, resource allocation, automatic
host-port selection, configurable service dependency ordering, custom-image
CLI commands, and general container orchestration are outside this change.

## Decisions

### Catalog and deployment ownership

Embed `sidecar-catalog.yaml` alongside the SLC catalog. Its versioned shape is:

```yaml
version: 1
sidecars:
  example:
    description: Example service
    architectures: [amd64, arm64]
    container:
      name: example
      image: "<approved-image>@sha256:<digest>"
      imagePullPolicy: IfNotPresent
      restartPolicy: OnFailure
      ports:
        - containerPort: 8080
          hostPort: 18080
          hostIP: 127.0.0.1
```

Catalog keys are DNS labels. `database` is reserved. A missing catalog
`container.name` defaults to its key; an explicit name matches the key.
Architecture support describes the container target, including Linux guests
on macOS and Windows. Prefer a pinned multi-architecture image reference.

Enable copies the selected template into `<deployment-dir>/sidecars.yaml`:

```yaml
version: 1
containers:
  - name: example
    image: "<approved-image>@sha256:<digest>"
    imagePullPolicy: IfNotPresent
    restartPolicy: OnFailure
```

Presence in `containers` means enabled. Read and validate the entire document
before making changes, and replace it atomically under the deployment lock.
Use restricted file permissions consistent with deployment-owned secrets,
because users can add literal environment values.

The catalog supplies initial values. Repeated enable preserves the saved
definition. A launcher upgrade leaves enabled definitions authoritative,
including their pinned image and user edits. Lifecycle operations continue to
manage saved entries even if a later catalog drops them. New enable requests
resolve against the current catalog. The CLI operates on names, while direct
edits to supported Container fields are honored at reconciliation.
Load the catalog for listing and new template selection. Saved names, including
user-added or renamed entries, drive subsequent operations. Disable also
accepts a name already absent from the saved document so cleanup can be retried.

### Supported Container fields

The initial field set is:

| Field | Supported values or behavior |
| --- | --- |
| `name` | Unique DNS label, with `database` reserved |
| `image` | Concrete image reference |
| `imagePullPolicy` | `Always`, `IfNotPresent`, `Never` |
| `command`, `args` | Kubernetes entrypoint and argument arrays |
| `workingDir` | Container working directory |
| `env` | `name`, `value`, or `valueFrom.secretKeyRef` |
| `ports` | `name`, `containerPort`, `hostPort`, `hostIP`, TCP |
| `restartPolicy` | `Always`, `OnFailure`, `Never` |

Use strict decoding and field-path errors for unsupported fields and values.
Validate duplicate names, mutually exclusive environment sources, and valid
port ranges. Apply Kubernetes defaults where applicable; use `OnFailure` as
the launcher's default restart policy and serialize it when enabling.

Expand `$(NAME)` in command and argument strings from the resolved container
environment using Kubernetes semantics, including `$$` escaping and unchanged
unresolved references. Literal environment values use Kubernetes ordered
expansion semantics. A missing value and source represent an empty value.
Tests cover image defaults when command or arguments are omitted.

Individual Container restart policies require compatible Kubernetes feature
support when Kubernetes execution is introduced. The current Podman mapping
uses `always`, `on-failure`, and `no`. Explicit lifecycle stop and removal
take precedence over automatic process restart.

### Runtime connection references

Use standard environment references:

```yaml
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

The launcher-owned `exasol-database` source provides `host`, `port`, `username`,
and `password`. Host is the `database` alias; port is its internal SQL port.
Read username and password from the existing deployment connection and secret
sources. Environment variable names remain freely selectable.

Use `enable <name> --no-db-password` as the proposed opt-out spelling. It omits
password references on initial enable and removes such references from an
already-enabled definition when explicitly supplied. Ordinary enable preserves
an existing omission. Users can also remove references directly.

Honor `secretKeyRef.optional`: omit an unavailable optional value and report
the source and key for an unavailable required value. Resolve references when
creating containers. A deployment restart recreates them with current values.
Live credential rotation is outside the lifecycle contract.

Pass every resolved environment value through optional child-process
environment overrides on the execution environment's `Run` method. Podman
arguments use `--env NAME` for literal values and connection references alike.
Direct execution sets the child environment. Runner and SSH adapters transport
overrides through stdin, preserving the command's own input and argument
boundaries. Empty values override inherited values; omitted optional references
produce no environment override or Podman environment flag.
The Podman process receives these overrides too. Catalog entries use
application-specific names; edits to variables that configure Podman also
affect that process.
Runtime diagnostics redact resolved secrets and literal sensitive
configuration. Saved definitions retain references. A user
who explicitly expands a secret into application arguments or output controls
that application behavior; launcher diagnostics still apply redaction.

### Networking and publication

Local deployments create one named DNS-enabled Podman bridge network in the
execution environment. Use deployment-qualified runtime names and ownership
labels; assign network aliases `database` and each saved sidecar name.
Independent containers allow services and port mappings to change while the
database stays running. Pod publication would couple mapping changes to pod
recreation.

Attach an existing bridge-backed database to the managed network when
necessary. New database containers join it during creation, including when
the enabled sidecar set is empty. Preserve database endpoint publication.
For an existing running non-bridge database, save enablement and return
`restartRequired: true` with enabled, stopped sidecar state. Text output
includes the `exasol stop` then `exasol start` call to action. JSON exposes
the restart requirement and follows the existing CTA suppression rules.
The next explicit stop/start recreates the database on the bridge.

`containerPort` declares the internal port. A positive `hostPort` requests
publication on `hostIP`. Local publication uses loopback, defaulting an omitted
host IP to `127.0.0.1`. Validation reports a non-loopback binding as
unsupported.
Keep host ports explicit and fixed. Ordinary bind errors identify the sidecar
and port. Status reports effective host IP and port mappings.

Platform runtimes translate host publication into the necessary guest mapping
and host forward. Guest ports are runtime details. Track mappings sufficiently
to remove only the selected sidecar's forwards on disable. The deployment
network remains while the database uses it and is removed on destroy.

Cloud sidecars use host networking on every node. The database uses the host
network through COS. Supply `database` in each sidecar's hosts file using the
node's private address. Loopback can be used when SQL loopback reachability is
verified. The application controls its listening address and port. Host mode
requires a declared `hostPort` to equal `containerPort`; it performs no port
translation and does not enforce `hostIP` or internal-only exposure. Catalog
commands and environment configure the application's bind address. Published
endpoint information describes the declared service endpoint on that node.
Bridge aliases and database-to-sidecar DNS are local-network capabilities.

### Sidecar hosts

The deployment backend supplies one local host or the sorted cloud-node set.
Each host supplies its identity, target architecture, availability, database
connection values, and container manager. The Podman manager shares creation,
inspection, definition comparison, and cleanup across command transports.
Sidecar host discovery is part of the backend contract, and local runtimes
provide sidecar management. Command transports share the `CommandRunner`
contract with installation execution environments.
Cloud commands use the existing SSH transport, with environment overrides sent
through private standard input. Architecture selection uses the container
host rather than the machine running the launcher. The built-in cloud
installation targets AMD64; local hosts use their runtime architecture.
Runtime ownership and applied-definition labels use `com.exasol.launcher`.

Desired state remains in one deployment-wide `sidecars.yaml`. Observed state
is queried per host. Status returns `name`, `enabled`, and a `hosts` array;
each entry contains the host name, running state, exit code, failure, restart
requirement, and endpoints. Local deployments use the host name `local`.
Before cloud provisioning, the host array is empty. Persist operation
failures in launcher-managed `sidecars-state.json`, keyed by host and sidecar.
Inspect runtime containers for current state and endpoints. Report per-host
`reconciliation` as `complete`, `pending`, or `unknown`. For an enabled sidecar
with a running database, completion requires one running container with the
saved-definition hash and matching published mappings, including VM forwards.
For a disabled sidecar or stopped database, completion requires owned
containers and forwards to be absent. An unavailable host or failed inspection
leaves reconciliation unknown.

Report unresolved recorded diagnostics as `lastOperationError`; `error`
describes a current inspection failure. Status takes the exclusive deployment
lock and clears recorded failures only for hosts whose desired outcome is
verified. It updates the advisory file atomically when records change and
preserves records for other names and hosts. Inspection leaves containers,
networks, forwards, and desired definitions untouched. Successful mutations
also clear their recorded errors. A failed node allows attempts on other nodes;
retries reconcile each node independently.

### Lifecycle hooks

One sidecar service owns enablement, reconciliation, inspection, cleanup, and
per-host failures. Backends supply host discovery and transport. Networking
and forwarding belong to host adapters. Lifecycle integration uses three
encapsulated, sidecar-specific hooks:

- `HostsReady`: reconcile after local runtime startup or cloud installation,
  after cloud nodes restart, and on an already-running start. Local and cloud
  restart paths invoke it before their database-readiness wait.
- `BeforeStop`: attempt cleanup on all reachable hosts while command execution
  is available. Database stop proceeds even when sidecar cleanup fails.
- `AfterDestroy`: clear observed failure records after successful destruction.
  Desired definitions remain available for redeployment.

Initialization and stopped enablement only save intent. Cloud destruction
removes the nodes and their containers; local destruction removes owned
containers before destroying the runtime. Hooks preserve the distinction
between database lifecycle errors and sidecar failures.

### Lifecycle and reconciliation

Use exclusive deployment locks for sidecar mutations, status refresh, and
lifecycle operations. Catalog listing uses the shared lock.
Enabled definitions follow the deployment's desired running state. Services
own connection retries; their startup is independent of SQL readiness.

- Enable while running persists intent and starts the selected saved
  definition when bridge networking is available. Legacy non-bridge networking
  reports the explicit restart requirement.
- Enable while stopped records intent for the next start.
- Repeated enable ensures the saved sidecar exists while preserving its edits.
- Start reconciles saved definitions even when the database is already ready.
- Unchanged running sidecars stay running. Changed definitions are recreated.
- Stop removes disposable sidecar containers and forwards before stopping the
  execution environment, retaining enabled definitions for the next start.
- Disable removes the selected definition, container, and owned runtime state.
- Destroy removes all sidecar runtime resources and the deployment network.

Initialized deployments can record enablement, which deploy or install
applies when starting the database. Cloud hosts become available after
provisioning, and start refreshes the node set before reconciliation.

Store a hash of the defaulted saved Container definition in an ownership-scoped
container label. Reconciliation compares that label with the desired hash and
checks live container presence and running state. Resolved connection values
are runtime inputs; credential changes take effect on recreation. The label
is the applied-definition contract rather than a field-by-field runtime diff.
Reconciliation runs during commands and lifecycle operations. It retries
missing or failed containers and cleans up owned containers removed from the
saved document.
Write enable intent before creation; write disable intent before cleanup so
retry can complete either interrupted operation. Cleanup can identify resources
from ownership labels even when the definition has already been removed.
Recorded cleanup hints supplement live discovery. Retry cleanup with
`exasol sidecar disable <name>` or `exasol start` when hosts are reachable.

Return sidecar failures with their name and cause. Retain database state based
on database behavior and attempt cleanup of all owned sidecars during stop.
Successful cleanup reconciles observed state; failed cleanup remains visible.
This extends orchestration around the database backend, keeping sidecar errors
distinct from existing database lifecycle failure transitions.

Status returns deployment enablement and per-host running state, reconciliation
outcome, runtime exit or startup error, and service endpoints. Reconciliation
completion verifies runtime configuration and publication; application-level
health remains the service's responsibility.
Commands render text or structured JSON from the same domain results using
the existing output helpers. Errors and recorded diagnostics describe failures;
the command layer queues recovery guidance through the CTA helper, including
lifecycle failures. Sidecar actions use deployment-file logging. Progress
remains on stderr and primary command results remain on stdout.

### Linux host-shell access

For Linux local deployments, `exasol shell host` runs the shell named by
`SHELL`, falling back to `/bin/sh` when unset or empty. Arguments following
`--` execute directly with their original argument boundaries. Both forms
inherit the caller's environment, working directory, and standard streams.
Execution uses the command context and surfaces process startup and exit
failures. Host access is available for initialized and stopped deployments.

Sidecar inspection uses explicit host commands through this same CLI boundary.
Linux container-shell access and Windows shell behavior retain their existing
contracts.

### Runtime integration prerequisite

macOS uses the runner's `forward add` and `forward remove` porcelain commands
for immediate, idempotent publication changes while the database runs. Its
public state supplies the effective host mappings. Sidecar-owned names isolate
cleanup from database forwards. Podman allocates guest publication ports;
saved definitions retain the requested host mappings.

Development builds can override the embedded runner resource with a local
build containing this contract. Release builds require a compatible released
runner resource pin. Companion runner changes belong to that repository.
Windows uses its existing Podman machine transport and needs host-loopback
verification in its deployment lane.

### Validation

Use a pinned official Caddy Alpine image through a test-only embedded catalog.
Supply that catalog in a test build through the same catalog-loading seam as
production. Keep fixture source under the existing test tree and let the test
build select it; the production binary exposes the built-in catalog API.

Run `caddy respond --listen :8080 --body <text>` with environment references in
the body. Return host, port, username, password, and a literal `TEST_MARKER`.
The password is randomly generated for the disposable test deployment. Compare
the response with the fixture's expected values. An opt-out check inspects
variable presence. Change the marker and run start to verify reconciliation.

Use runtime execution to test database DNS and TCP connectivity from Caddy and
sidecar DNS and HTTP connectivity from the database network context. Exercise
process failure from inside the container: `podman stop` and `podman kill`
intentionally suppress restart and are unsuitable for the failure-policy test.

Unit and CLI tests cover parsing, defaults, errors, reference resolution, and
command results. Real deployment tests prove networking, publication,
lifecycle, and recovery on the existing Linux AMD64, macOS ARM64, and Windows
AMD64 lanes. Each spec scenario has exactly one named owning test; repeated
platform execution is the same test. Additional implementation tests remain
clearly supplementary. The scenario-to-test map is maintained in tasks.md.

Cloud deployment tests use the existing shared real-deployment fixture and
probe each node through host commands. Configure Caddy to bind to loopback,
with equal host and container ports. Verify the reported endpoint, resolved
connection values, database alias, definition reconciliation, and cleanup on
every node. Unit tests keep deployment and runtime dependencies isolated.

## Risks / Trade-offs

- Named-network attachment can affect existing DNS or UDF traffic. Verify
  database queries and the existing local deployment suite after attachment.
- VM forwarding may require a runner release. Keep live forwarding as an
  explicit dependency and verify it before changing the resource pin.
- User edits can change image behavior and credentials handling. Validate the
  supported schema and document deployment ownership and loopback publication.
- Fixed host ports can be occupied. Surface the runtime bind error; users can
  edit the saved mapping and retry start.
- Environment and argument expansion can surface secrets in application
  behavior. Protect launcher-owned delivery and redact launcher diagnostics.
- A stopped shared Windows container host can retain old containers. Reconcile
  pending removal before starting enabled services when the host returns.

## Migration Plan

A missing `sidecars.yaml` represents an empty enabled set. Every new database
container joins the deployment bridge. Existing non-bridge deployments migrate
on their next explicit stop/start. Catalog upgrades affect new enablement;
saved definitions remain authoritative. To roll back the launcher, disable
enabled sidecars with the supporting version first so containers and forwards
are cleaned up.
