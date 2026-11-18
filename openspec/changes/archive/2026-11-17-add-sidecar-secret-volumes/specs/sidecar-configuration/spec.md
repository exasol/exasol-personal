## MODIFIED Requirements

### Requirement: Container configuration has a validated supported subset

The launcher SHALL support `name`, `image`, `imagePullPolicy`, `command`,
`args`, `workingDir`, `env`, `ports`, `restartPolicy`, `readinessProbe`, and
`volumeMounts`.
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

## ADDED Requirements

### Requirement: Secret volumes deliver connection values as files

The catalog and `sidecars.yaml` SHALL support `volumes` beside the `container`
of a `sidecars.<name>` entry. Each volume SHALL have a `name` and a `secret`
with `secretName`, optional `items` of `key` and `path`, and `optional`. A
container's `volumeMounts` SHALL name a volume and carry an absolute
`mountPath` and `readOnly`. Before the sidecar's process starts, each item
SHALL be readable in the container at its `path` below the mount, by any user
the container runs as, holding the value of its key, and the files and their
directories SHALL be read-only to every user other than `root`. A volume without `items`
SHALL provide every key of its source, each at a path equal to its key. An
unavailable key SHALL fail startup, unless the volume is `optional`, which
omits that file. Container inspection SHALL show neither the values a secret
volume delivers nor values derived from them. The launcher SHALL report a
volume without a secret source, a duplicate volume name, an item path that is
absolute or contains `..`, a mount naming an undeclared volume, and a mount
path that is relative, `/`, or repeated as a validation error naming the
offending field.

#### Scenario: A secret volume delivers the password as a file

- **WHEN** a running sidecar mounts a secret volume holding the `password` key
  of `exasol-database`
- **THEN** its process reads the current password from the file at the item's
  path below the mount
- **AND** `podman inspect` or `docker inspect` of the sidecar contains the
  password nowhere

#### Scenario: A volume without items provides every key

- **WHEN** a sidecar mounts a secret volume that lists no items
- **THEN** each key of its source is a file named after that key below the
  mount

#### Scenario: Invalid volume declaration names the offending field

- **WHEN** a sidecar operation reads a volume or volume mount that breaks a
  validation rule
- **THEN** it fails validation and names the offending configuration field
