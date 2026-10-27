## ADDED Requirements

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

## REMOVED Requirements

### Requirement: Saved definitions are authoritative

**Reason**: Disabling a name the deployment has not saved succeeds and reports
that there is nothing to remove, so a repeated removal succeeds.

**Migration**: The requirement continues as "Saved sidecar definitions are
authoritative".
