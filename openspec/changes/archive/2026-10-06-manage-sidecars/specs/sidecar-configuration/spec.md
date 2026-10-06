## Purpose

Define how users select catalog sidecars, retain editable deployment
configuration, and supply runtime database connection values.

## ADDED Requirements

### Requirement: Users can discover supported sidecars

`exasol sidecar list` SHALL report catalog names, descriptions, architecture
support, and enablement for the selected deployment in text and JSON formats.
New enable requests SHALL resolve a supported catalog name.

#### Scenario: Catalog listing describes deployment availability

- **WHEN** the user lists sidecars for a deployment with an enabled entry
- **THEN** the output identifies the catalog entries, architecture support,
  and which entry is enabled in both text and JSON formats

#### Scenario: Unsupported selection explains the available choices

- **WHEN** the user enables an unknown name or an architecture-incompatible
  catalog entry
- **THEN** the command fails with the selection error and supported choices

### Requirement: Saved definitions are authoritative

The launcher SHALL store enabled definitions in a versioned `sidecars.yaml`
with top-level `containers` using Kubernetes Container field names. It SHALL
accept edits to supported fields and use those values for subsequent starts.

#### Scenario: First enable materializes the template

- **WHEN** the user enables a catalog entry for the first time
- **THEN** its resolved named Container definition appears exactly once in
  `sidecars.yaml`, including its concrete image and default restart policy

#### Scenario: Repeated enable preserves deployment edits

- **WHEN** a saved definition differs from its catalog template and the user
  enables that sidecar again
- **THEN** the saved field values remain the definition used for that sidecar

#### Scenario: Catalog changes preserve enabled definitions

- **WHEN** the launcher catalog changes or drops an already-enabled entry
- **THEN** the deployment's saved definition remains available for start,
  status, and disable operations

### Requirement: Container configuration has a validated supported subset

The launcher SHALL support `name`, `image`, `imagePullPolicy`, `command`,
`args`, `workingDir`, `env`, `ports`, and `restartPolicy`. Environment entries
SHALL support literal values and `valueFrom.secretKeyRef` with `name`, `key`,
and `optional`. Ports SHALL support `name`, `containerPort`, `hostPort`,
`hostIP`, and TCP protocol. Image pull policies SHALL support `Always`,
`IfNotPresent`, and `Never`. Restart policies SHALL support `Always`,
`OnFailure`, and `Never`, defaulting to `OnFailure`.

The launcher SHALL report invalid documents, unsupported fields or values,
duplicate container or environment names, and conflicting environment sources
with an error identifying the offending location. Container names SHALL be
unique DNS labels with `database` reserved for the database.

#### Scenario: Supported configuration determines container execution

- **WHEN** a saved definition supplies supported command, arguments, working
  directory, environment, image pull policy, and restart policy values
- **THEN** the container runs with the requested settings

#### Scenario: Invalid configuration identifies the offending location

- **WHEN** a sidecar operation reads malformed or unsupported configuration
- **THEN** it fails validation and identifies the offending document location

### Requirement: Environment references resolve at container creation

The launcher SHALL resolve the `exasol-database` source keys `host`, `port`,
`username`, and `password` for each container creation. Host SHALL be
`database`, port SHALL be its internal SQL port, and credentials SHALL come
from the selected deployment. Saved definitions SHALL retain references.
Required unavailable references SHALL produce a source-and-key error;
unavailable optional references SHALL produce an absent environment variable.

#### Scenario: Connection values and literal environment reach the container

- **WHEN** a sidecar is created with all four connection references and a
  literal environment value
- **THEN** its environment contains the selected deployment's current
  connection values and the literal value under the configured variable names
- **AND** the saved connection entries remain references

#### Scenario: Required connection reference is unavailable

- **WHEN** a container definition references an unavailable required source
  or key
- **THEN** the operation reports the sidecar, source, and key that failed

#### Scenario: Optional connection reference is unavailable

- **WHEN** a container definition references an unavailable source or key with
  `optional: true`
- **THEN** the container is created with that environment variable absent

### Requirement: Password injection is selectable

`enable <name> --no-db-password` SHALL save the definition with database
password references omitted. Removing such a reference by editing the saved
definition SHALL have the same effect on the next reconciliation.

#### Scenario: Password opt-out persists through restart

- **WHEN** a user enables a sidecar with `--no-db-password` and later restarts
  the deployment
- **THEN** the saved definition and recreated container environment both
  reflect the password opt-out

#### Scenario: Edited password opt-out is honored

- **WHEN** a user removes a database password reference from an enabled
  sidecar's saved definition and runs start
- **THEN** the reconciled container has that environment variable absent

### Requirement: Argument expansion follows Kubernetes semantics

The launcher SHALL expand environment references in command and argument
arrays using the resolved container environment, preserve unresolved
references literally, and interpret `$$` as an escaped dollar sign. Literal
environment values SHALL use Kubernetes ordered expansion semantics.

#### Scenario: Expansion preserves Kubernetes escaping and ordering

- **WHEN** a definition uses environment references, escaped dollars,
  unresolved references, and references to earlier environment entries
- **THEN** the resulting environment, command, and arguments follow the
  corresponding Kubernetes expansion rules

### Requirement: Launcher diagnostics protect connection credentials

Launcher output and diagnostics SHALL represent sensitive resolved values
with redacted text. Launcher-created configuration SHALL use restricted
access appropriate to deployment credentials.
Resolved environment values SHALL reach the container through child-process
environment overrides, with variable names in Podman's environment arguments.

#### Scenario: Failure diagnostics redact injected credentials

- **WHEN** sidecar startup fails after connection credentials are resolved
- **THEN** launcher diagnostics identify the sidecar and failure cause with
  credential values redacted
- **AND** launcher-created credential-bearing files have restricted access
- **AND** Podman's environment arguments contain only variable names
