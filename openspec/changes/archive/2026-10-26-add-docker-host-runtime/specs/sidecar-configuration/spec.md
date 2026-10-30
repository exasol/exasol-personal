## MODIFIED Requirements

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
