## MODIFIED Requirements

### Requirement: CLI SHALL list known deployment directories
`exasol deployments list` SHALL enumerate default and named deployment
directories at the new managed-deployments root and include a deployment at
the legacy root when its name has no matching path at the new root. Each name
SHALL appear once, with the new path taking precedence when both locations
contain it. The root locations are specified by
`deployment-directory-resolution`.

#### Scenario: Listing shows the default deployment directory
- **WHEN** a user runs `exasol deployments list`
- **AND** a `default` deployment directory exists under the new
  managed-deployments root
- **THEN** the output includes an entry for `default` with its resolved path

#### Scenario: Listing shows named deployment directories
- **WHEN** a user runs `exasol deployments list`
- **AND** one or more named deployment directories exist under the new
  managed-deployments root
- **THEN** the output includes one entry per named deployment directory, with
  its name and resolved path

#### Scenario: Listing shows a kept legacy default deployment
- **WHEN** a `default` deployment remains at the legacy root
- **AND** no matching path exists at the new root
- **THEN** `exasol deployments list` includes `default` with its legacy path

#### Scenario: Listing shows a kept legacy named deployment
- **WHEN** a named deployment remains at the legacy root
- **AND** no matching path exists at the new root
- **THEN** `exasol deployments list` includes its name and legacy path

#### Scenario: Listing shows the selected location on a collision
- **WHEN** paths with the same name exist at the new and legacy roots
- **THEN** `exasol deployments list` includes one entry for that name with the
  new path

#### Scenario: Listing is empty when no deployment directories exist
- **WHEN** a user runs `exasol deployments list`
- **AND** neither root exists or contains a deployment directory
- **THEN** the command succeeds
- **AND** the output indicates that no deployment directories exist

#### Scenario: Non-directory entries are ignored
- **WHEN** a user runs `exasol deployments list`
- **AND** either root contains a non-directory entry (a file or symlink)
- **THEN** the output does not include an entry for it
- **AND** the command does not fail because of it

#### Scenario: Listing is sorted alphabetically by name
- **WHEN** a user runs `exasol deployments list`
- **AND** more than one deployment directory is selected from the two roots
- **THEN** the entries appear in alphabetical order by name, regardless of
  filesystem iteration order
