# sidecar-configuration Specification

## Purpose

Define how users select catalog sidecars and retain editable deployment
configuration for the services that run beside a database.

## Requirements

### Requirement: Users can discover supported sidecars

`exasol sidecar list` SHALL report catalog names, descriptions, architecture
support, and enablement for the selected deployment in text and JSON formats.
Without an initialized deployment it SHALL report the catalog with the current
machine's architecture support and no entry enabled. New enable requests SHALL
resolve a supported catalog name.

#### Scenario: Catalog listing describes deployment availability

- **WHEN** the user lists sidecars for a deployment with an enabled entry
- **THEN** the output identifies the catalog entries, architecture support,
  and which entry is enabled in both text and JSON formats

#### Scenario: Listing reflects the selected deployment's type

- **WHEN** the user lists sidecars for a deployment that cannot run them
- **THEN** every entry reports itself unsupported and says why

#### Scenario: Catalog listing answers before a deployment exists

- **WHEN** the user lists sidecars without an initialized deployment
- **THEN** the output identifies the catalog entries with the current machine's
  architecture support and reports no entry as enabled

#### Scenario: Unsupported selection explains the available choices

- **WHEN** the user enables an unknown name or an architecture-incompatible
  catalog entry
- **THEN** the command fails with the selection error and supported choices

### Requirement: Saved sidecar definitions are authoritative

The launcher SHALL store saved definitions in a versioned `sidecars.yaml`
with a top-level `sidecars` map keyed by sidecar name, where each entry holds a
`container` using Kubernetes Container field names. The entry's key SHALL name
its container, and a `name` inside `container` SHALL match that key. It SHALL
accept edits to supported fields and use those values for subsequent
operations. Enable, status, and disable SHALL report the deployment's
enablement and operate on saved names. Status without a name SHALL report every
saved sidecar in name order. Status SHALL reject a name that is neither saved
nor offered by the catalog.

#### Scenario: First enable materializes the template

- **WHEN** the user enables a catalog entry for the first time
- **THEN** its resolved container appears exactly once in `sidecars.yaml` under
  that entry's name, including its concrete image and default restart policy,
  with `enabled: true`

#### Scenario: The entry key names its container

- **WHEN** a saved entry omits `name` inside its `container`
- **THEN** operations identify that container by the entry's key

#### Scenario: A container name that contradicts its key fails validation

- **WHEN** a saved entry carries a `container.name` that differs from its key
- **THEN** the operation fails validation and names the offending field

#### Scenario: Repeated enable preserves deployment edits

- **WHEN** a saved definition differs from its catalog template and the user
  enables that sidecar again
- **THEN** the saved field values remain the definition used for that sidecar

#### Scenario: Catalog changes preserve enabled definitions

- **WHEN** the launcher catalog changes or drops an already-enabled entry
- **THEN** the deployment's saved definition remains available for status and
  disable operations

#### Scenario: Status without a name reports every saved sidecar

- **WHEN** the user asks for status without naming a sidecar
- **THEN** it reports one entry per saved definition, in name order

### Requirement: Saved entries record their enabled state

Each `sidecars.<name>` entry in `sidecars.yaml` SHALL support an `enabled`
field beside its `container`. Enable SHALL save `enabled: true` on the entry.
An entry without the field SHALL be enabled. List and status SHALL report an
entry as enabled only when it is saved and enabled. The catalog SHALL reject an
`enabled` field with a validation error naming the offending field.

#### Scenario: An entry without the field is enabled

- **WHEN** a user adds an entry without `enabled` to `sidecars.yaml`
- **THEN** list and status report that sidecar as enabled

#### Scenario: A disabled entry reports as disabled

- **WHEN** a saved entry carries `enabled: false`
- **THEN** list and status report that sidecar as disabled, and status without
  a name still includes it

#### Scenario: A catalog entry carrying enablement fails validation

- **WHEN** a catalog entry carries an `enabled` field
- **THEN** loading the catalog fails and names the offending field

### Requirement: Container configuration has a validated supported subset

The launcher SHALL support `name`, `image`, `imagePullPolicy`, `command`,
`args`, `workingDir`, `env`, `ports`, `restartPolicy`, and `readinessProbe`.
Environment entries SHALL support literal values and `valueFrom.secretKeyRef`
with `name`, `key`, and `optional`. Ports SHALL support `name`,
`containerPort`, `hostPort`, `hostIP`, and TCP protocol. Image pull policies
SHALL support `Always`, `IfNotPresent`, and `Never`. Restart policies SHALL
support `Always`, `OnFailure`, and `Never`, defaulting to `Always`.

The launcher SHALL reject documents it cannot parse and documents carrying
unsupported fields. It SHALL report unsupported values, duplicate environment
names, and conflicting environment sources with an error naming the offending
field. Container names SHALL be DNS labels with `database` reserved for the
database.

#### Scenario: Unsupported configuration is rejected

- **WHEN** a sidecar operation reads a document it cannot parse or one carrying
  an unsupported field
- **THEN** it fails without applying any part of the document

#### Scenario: Invalid values name the offending field

- **WHEN** a sidecar operation reads an unsupported value, a duplicate
  environment name, or conflicting environment sources
- **THEN** it fails validation and names the offending configuration field

#### Scenario: Broad publication request receives a validation error

- **WHEN** a saved sidecar requests a non-loopback host IP
- **THEN** validation identifies that mapping and the supported loopback scope

### Requirement: Sidecars declare an application protocol for a named port

The catalog and `sidecars.yaml` SHALL support `endpoints` declarations beside
the `container` of a `sidecars.<name>` entry. Each declaration SHALL name one of
that container's ports and SHALL carry the application protocol scheme that port
speaks, with an optional path. The launcher SHALL save an enabled sidecar's
declarations under its entry and SHALL accept edits to them. It SHALL report a
declaration naming a port the container does not declare, a missing or
malformed scheme, or a path that does not begin with `/` as a validation error
naming the offending field.

#### Scenario: Enabling saves the declared endpoints

- **WHEN** the user enables a catalog entry that declares an endpoint for one of
  its ports
- **THEN** `sidecars.yaml` carries that declaration alongside the saved
  definition

#### Scenario: Catalog changes preserve declared endpoints

- **WHEN** the launcher catalog changes or drops an already-enabled entry
- **THEN** the deployment's saved declarations remain the ones status reports

#### Scenario: Edited declarations apply without recreating the container

- **WHEN** the user edits the scheme or path of a saved declaration while its
  sidecar runs
- **THEN** status reports the edited values and reconciliation stays complete

#### Scenario: Invalid declaration names the offending field

- **WHEN** a declaration names a port the container does not declare, omits its
  scheme, or gives a path that does not begin with `/`
- **THEN** the operation fails validation and names the offending field

### Requirement: Saved definitions retain database connection references

Saved definitions SHALL retain `valueFrom.secretKeyRef` entries naming the
launcher-owned `exasol-database` source, whose keys are `host`, `port`,
`username`, and `password`. Launcher diagnostics SHALL withhold resolved
password values and SHALL report the other connection values.

#### Scenario: Diagnostics withhold the password and keep other values

- **WHEN** a sidecar failure reports diagnostics that contain resolved
  connection values
- **THEN** the message withholds the password and still shows the other values

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

### Requirement: Containers declare a readiness probe

A container's `readinessProbe` SHALL support exactly one handler: `exec` with
a non-empty `command`, `httpGet` with `port`, `path`, and `scheme` of `HTTP` or
`HTTPS`, or `tcpSocket` with `port`. It SHALL support `initialDelaySeconds`,
`periodSeconds`, `timeoutSeconds`, and `failureThreshold`, defaulting to 0, 10,
1, and 3 when omitted or 0. An `httpGet` path SHALL default to `/` and its
scheme to `HTTP`. An `httpGet` or `tcpSocket` port SHALL name, by number or by
port name, a container port with a positive `hostPort`. The launcher SHALL
report a probe with no handler or several, a negative timing value, a path that
does not begin with `/`, an unsupported scheme, or a port that is not a
published container port as a validation error naming the offending field.
Editing the readiness probe of a running sidecar SHALL leave its container in
place and SHALL apply on the next status.

#### Scenario: Enabling saves the declared probe with its defaults

- **WHEN** the user enables a catalog entry that declares a readiness probe
- **THEN** `sidecars.yaml` carries that probe with its default timing values

#### Scenario: Invalid probe names the offending field

- **WHEN** a saved probe declares two handlers, a negative timing value, a path
  without a leading `/`, or a network handler whose port is not published
- **THEN** the operation fails validation and names the offending probe field

#### Scenario: Edited probe applies without recreating the container

- **WHEN** the user edits a running sidecar's readiness probe
- **THEN** the next status evaluates the edited probe, the same container keeps
  running, and reconciliation stays complete
