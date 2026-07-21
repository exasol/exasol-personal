## MODIFIED Requirements

### Requirement: Launcher-owned local runtime

The system SHALL own the local VM runtime files, VM disk/data, and managed deployment share inside the deployment directory.

#### Scenario: Embedded runner is staged

- **WHEN** the launcher initializes or starts a local deployment without an installed runner
- **THEN** it atomically writes the embedded macOS Exasol Local runner into the launcher-owned runtime directory before invoking it

#### Scenario: Unversioned legacy runner is upgraded

- **WHEN** the launcher prepares a stopped local deployment whose installed runner does not report a valid semantic version
- **THEN** it atomically replaces that runner with the versioned embedded migration runner before invoking it

#### Scenario: Compatible runner update is applied

- **WHEN** the embedded runner is a newer patch or minor version within the installed runner's major version
- **THEN** the launcher atomically replaces the installed runner before starting the local deployment

#### Scenario: Release-candidate runner update is applied

- **WHEN** a `v`-prefixed embedded runner release candidate has greater semantic precedence than the installed release candidate within the same major version
- **THEN** the launcher atomically replaces the installed runner before starting the local deployment

#### Scenario: Runner is not downgraded

- **WHEN** the installed runner is newer than the embedded runner within the same major version
- **THEN** the launcher retains the installed runner

#### Scenario: Same-version runner content differs

- **WHEN** the installed and embedded runners report the same semantic version but contain different bytes
- **THEN** the launcher replaces the installed runner with the trusted embedded runner before starting the local deployment

#### Scenario: Major runner update requires user action

- **WHEN** the installed and embedded runners report different major versions
- **THEN** the launcher retains the installed runner and informs the user that major runner updates are not automatic

#### Scenario: Active runner is not replaced

- **WHEN** the launcher performs status, stop, or destroy behavior for a local deployment
- **THEN** it does not replace an existing runner before invoking the lifecycle behavior

#### Scenario: Embedded runner version is invalid

- **WHEN** the embedded runner does not report a valid semantic version during preparation
- **THEN** the launcher fails before replacing or starting the installed runner

#### Scenario: Internal forced-reconciliation bypass is enabled

- **WHEN** development explicitly enables forced reconciliation with a differing unversioned embedded runner
- **THEN** the launcher atomically installs the embedded runner without version compatibility checks and warns that reconciliation was forced

#### Scenario: Runner VM sizing is prepared

- **WHEN** the launcher initializes or starts a local deployment
- **THEN** it exposes VM CPU, VM memory, and Exasol Local data disk sizing through the runner start command

#### Scenario: Managed share is prepared

- **WHEN** the launcher initializes a local deployment
- **THEN** it creates a launcher-managed share for guest coordination and SSH key import

#### Scenario: User shares are not exposed

- **WHEN** the user initializes or starts a local deployment
- **THEN** the launcher does not require or expose user-configurable shared folder settings
