# sidecar-configuration Specification

## Purpose

Define how users select catalog sidecars and retain editable deployment
configuration for the services that run beside a database.

## Requirements

### Requirement: Users can discover supported sidecars

`exasol sidecar list` SHALL report catalog names, descriptions, architecture
support, enablement, and host ports for the selected deployment in text and
JSON formats. Host ports SHALL be reported as comma-separated
`<port-name>:<host-port>` entries: the saved host ports for a sidecar with a
saved definition and the catalog template's host ports otherwise. Without an
initialized deployment it SHALL report the catalog with the current machine's
architecture support and no entry enabled. New enable requests SHALL resolve a
supported catalog name.

#### Scenario: Catalog listing describes deployment availability

- **WHEN** the user lists sidecars for a deployment with an enabled entry
- **THEN** the output identifies the catalog entries, architecture support,
  and which entry is enabled in both text and JSON formats

#### Scenario: Listing reports host ports by port name

- **WHEN** the user lists sidecars for a deployment whose saved catalog
  sidecar has a host port other than its template's
- **THEN** that entry reports its saved host ports as
  `<port-name>:<host-port>` entries in both text and JSON formats

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
### Requirement: Unnamed published ports keep their host port

Host port selection and `--ports` SHALL address published ports by `name`. A
port with a positive `hostPort` and no `name` SHALL keep its saved host port,
SHALL count as taken when the launcher selects other host ports, and SHALL be
omitted from reported `<port-name>:<host-port>` entries.

#### Scenario: Reset keeps an unnamed host port

- **WHEN** a user runs `exasol sidecar config reset <name> ports` for a saved
  sidecar that also publishes a port without a `name`
- **THEN** `sidecars.yaml` keeps that port's host port and the command reports
  only the named ports

### Requirement: Enable selects free host ports for a new definition

When `exasol sidecar enable <name>` saves a catalog template without
`--ports`, the launcher SHALL save a concrete host port for each named
published port. Selection SHALL start at the template's `hostPort` and SHALL
follow the default-first wrapping order of local service port allocation. A
selected port SHALL be available on every loopback address resolved for
`localhost` and SHALL differ from the host ports saved for the database and
for the deployment's other sidecars. When no port is available, enable SHALL
fail, save no definition, and identify the sidecar and port name.

#### Scenario: Default sidecar port is available

- **WHEN** a user enables a catalog sidecar whose template host port is usable
- **THEN** `sidecars.yaml` saves the template host port for that port

#### Scenario: Taken default advances to a free port

- **WHEN** a user enables a catalog sidecar whose template host port is held by
  another process
- **THEN** `sidecars.yaml` saves the first following usable port
- **AND** status reports the sidecar's endpoint on that port

#### Scenario: Selection skips ports saved for other services

- **WHEN** a user enables a catalog sidecar whose template host port is saved
  for the database or another sidecar of the deployment
- **THEN** the launcher saves a different usable host port

### Requirement: Users select sidecar host ports explicitly

`exasol sidecar enable <name> --ports` and
`exasol sidecar config set <name> --ports` SHALL accept comma-separated
`<port-name>:<host-port>` entries, where each port name identifies a published
port of that sidecar. The launcher SHALL save each given host port exactly and
keep the saved host ports of unlisted ports. On an already-enabled sidecar,
`enable --ports` SHALL update its saved host ports as `config set` does. The
launcher SHALL reject, with an error naming the offending entry and saving
nothing, an entry that is not of the form `<port-name>:<host-port>`, names no
published port, repeats a port name, carries a host port outside `1` through
`65535`, or carries a host port saved for the database or another sidecar of
the deployment.

#### Scenario: Explicit host port is saved exactly

- **WHEN** a user runs `exasol sidecar enable <name> --ports <port-name>:<port>`
  for a sidecar that has no saved definition
- **THEN** `sidecars.yaml` saves that host port for the named port

#### Scenario: Enable with ports updates an enabled sidecar

- **WHEN** a user runs `exasol sidecar enable <name> --ports <port-name>:<port>`
  for an enabled sidecar with a different saved host port
- **THEN** `sidecars.yaml` saves the given host port and keeps the rest of the
  saved definition

#### Scenario: Invalid port entry names the offending entry

- **WHEN** a user passes `--ports` with an entry that lacks a port name, an
  unknown port name, an unpublished port, a repeated port name, an
  out-of-range port, or a host port saved for another service
- **THEN** the command fails, names the offending entry, and leaves
  `sidecars.yaml` unchanged

### Requirement: Sidecar config commands manage saved host ports

`exasol sidecar config get <name> [ports]` SHALL report a saved sidecar's host
ports as comma-separated `<port-name>:<host-port>` entries in text and JSON
formats. `exasol sidecar config reset <name> ports` and
`exasol sidecar config reset <name> --all` SHALL select a free host port for
every published port under the selection rules of enable, starting at the
catalog template's host port for that port name, or at the saved host port
when the catalog no longer offers that sidecar or port name. `config set` and
`config reset` SHALL report the resulting values. Each config command SHALL
fail for a name without a saved definition and SHALL tell the user to enable
the sidecar first.

#### Scenario: Config get reports saved host ports

- **WHEN** a user runs `exasol sidecar config get <name>` for a saved sidecar
- **THEN** text and JSON output report its saved host ports

#### Scenario: Config reset selects automatic host ports

- **WHEN** a user runs `exasol sidecar config reset <name> ports`
- **THEN** each published port saves the first usable host port from its
  catalog template's host port, and the command reports the saved ports

#### Scenario: Config reset for a sidecar the catalog dropped

- **WHEN** a user resets the ports of a saved sidecar that the catalog no
  longer offers
- **THEN** selection starts at each port's saved host port

#### Scenario: Config command for an unsaved sidecar fails

- **WHEN** a user runs a sidecar config command for a name without a saved
  definition
- **THEN** the command fails and tells the user to run
  `exasol sidecar enable <name>`
