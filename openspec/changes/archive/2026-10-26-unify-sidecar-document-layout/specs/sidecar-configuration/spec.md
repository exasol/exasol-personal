## MODIFIED Requirements

### Requirement: Saved definitions are authoritative

The launcher SHALL store enabled definitions in a versioned `sidecars.yaml`
with a top-level `sidecars` map keyed by sidecar name, where each entry holds a
`container` using Kubernetes Container field names. The entry's key SHALL name
its container, and a `name` inside `container` SHALL match that key. It SHALL
accept edits to supported fields and use those values for subsequent
operations. Enable, status, and disable SHALL report the deployment's
enablement and operate on saved names. Status without a name SHALL report every
saved sidecar in name order. Disable SHALL fail for a name the deployment has
not saved. Status SHALL reject a name that is neither saved nor offered by the
catalog.

#### Scenario: First enable materializes the template

- **WHEN** the user enables a catalog entry for the first time
- **THEN** its resolved container appears exactly once in `sidecars.yaml` under
  that entry's name, including its concrete image and default restart policy

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

#### Scenario: Disabling an unsaved sidecar fails

- **WHEN** the user disables a name the deployment has not saved
- **THEN** the command fails, names the saved sidecars, and leaves
  `sidecars.yaml` unchanged

### Requirement: Container configuration has a validated supported subset

The launcher SHALL support `name`, `image`, `imagePullPolicy`, `command`,
`args`, `workingDir`, `env`, `ports`, and `restartPolicy`. Environment entries
SHALL support literal values and `valueFrom.secretKeyRef` with `name`, `key`,
and `optional`. Ports SHALL support `name`, `containerPort`, `hostPort`,
`hostIP`, and TCP protocol. Image pull policies SHALL support `Always`,
`IfNotPresent`, and `Never`. Restart policies SHALL support `Always`,
`OnFailure`, and `Never`, defaulting to `OnFailure`.

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
