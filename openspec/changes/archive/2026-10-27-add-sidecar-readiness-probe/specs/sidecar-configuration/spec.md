## MODIFIED Requirements

### Requirement: Container configuration has a validated supported subset

The launcher SHALL support `name`, `image`, `imagePullPolicy`, `command`,
`args`, `workingDir`, `env`, `ports`, `restartPolicy`, and `readinessProbe`.
Environment entries SHALL support literal values and `valueFrom.secretKeyRef`
with `name`, `key`, and `optional`. Ports SHALL support `name`,
`containerPort`, `hostPort`, `hostIP`, and TCP protocol. Image pull policies
SHALL support `Always`, `IfNotPresent`, and `Never`. Restart policies SHALL
support `Always`, `OnFailure`, and `Never`, defaulting to `OnFailure`.

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

## ADDED Requirements

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
