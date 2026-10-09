## Purpose

Define how users select catalog sidecars and retain editable deployment
configuration for the services that run beside a database.

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
accept edits to supported fields and use those values for subsequent
operations. Enable, status, and disable SHALL report the deployment's
enablement and operate on saved names. Status SHALL report a `hosts` array,
which is empty while the deployment has no sidecar hosts.

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
- **THEN** the deployment's saved definition remains available for status and
  disable operations

#### Scenario: Repeated disable reports the sidecar disabled

- **WHEN** the user disables a saved sidecar twice
- **THEN** both commands report it disabled and its definition is absent from
  `sidecars.yaml`

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

#### Scenario: Invalid configuration identifies the offending location

- **WHEN** a sidecar operation reads malformed or unsupported configuration
- **THEN** it fails validation and identifies the offending document location

#### Scenario: Broad publication request receives a validation error

- **WHEN** a saved sidecar requests a non-loopback host IP
- **THEN** validation identifies that mapping and the supported loopback scope

### Requirement: Saved definitions retain database connection references

Saved definitions SHALL retain `valueFrom.secretKeyRef` entries naming the
launcher-owned `exasol-database` source, whose keys are `host`, `port`,
`username`, and `password`.

#### Scenario: Saved connection entries remain references

- **WHEN** a sidecar is enabled from a template that names connection keys
- **THEN** its saved definition stores the references rather than resolved
  values

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

#### Scenario: Required connection reference is unavailable

- **WHEN** a definition references an unavailable required source or key
- **THEN** resolution reports the sidecar, source, and key that failed

### Requirement: Launcher-owned configuration restricts access

Launcher-created files that can carry literal credential values SHALL use
access restricted to the deployment owner.

#### Scenario: Saved definitions are owner-readable only

- **WHEN** the launcher writes `sidecars.yaml`
- **THEN** the file grants access only to the deployment owner
