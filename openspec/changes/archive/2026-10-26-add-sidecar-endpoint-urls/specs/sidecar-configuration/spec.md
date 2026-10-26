## ADDED Requirements

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
