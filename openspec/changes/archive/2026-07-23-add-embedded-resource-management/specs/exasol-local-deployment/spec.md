## MODIFIED Requirements

### Requirement: Launcher-owned local runtime

The system SHALL own the local VM disk/data and managed deployment share inside the deployment directory, and SHALL resolve the Exasol Local VM runner through the resource manager on every use rather than maintaining a per-deployment copy of it.

#### Scenario: Runner is resolved without a per-deployment copy

- **WHEN** the launcher initializes or starts a local deployment
- **THEN** it resolves the Exasol Local runner through the resource manager and invokes it directly from the resolved location, without copying it into the deployment directory

#### Scenario: Missing version marker is initialized

- **WHEN** the launcher prepares a local deployment that has no persisted runner-version marker, or an invalid one, and the resolved runner reports a valid semantic version
- **THEN** it records the resolved runner's version as the deployment's persisted marker before invoking the runner

#### Scenario: Compatible runner update is recorded

- **WHEN** the resolved runner is a newer patch or minor version within the persisted marker's major version
- **THEN** the launcher updates the persisted marker to the resolved runner's version before starting the local deployment

#### Scenario: Release-candidate runner update is recorded

- **WHEN** a `v`-prefixed resolved runner release candidate has greater semantic precedence than the persisted marker's release candidate within the same major version
- **THEN** the launcher updates the persisted marker to the resolved runner's version before starting the local deployment

#### Scenario: Unsafe version relationship proceeds with a warning

- **WHEN** the resolved runner's version differs in major version from the persisted marker, or is older than the persisted marker within the same major version
- **THEN** the launcher proceeds using the resolved runner, logs a warning describing the version relationship, and updates the persisted marker to the resolved runner's version

#### Scenario: Version reconciliation is skipped for non-starting lifecycle behavior

- **WHEN** the launcher performs status, stop, or destroy behavior for a local deployment
- **THEN** it resolves and invokes the runner without comparing or updating the persisted version marker

#### Scenario: Resolved runner version is invalid

- **WHEN** the resolved runner does not report a valid semantic version during preparation
- **THEN** the launcher fails before invoking it, unless forced reconciliation is enabled

#### Scenario: Internal forced-reconciliation bypass is enabled

- **WHEN** development explicitly enables forced reconciliation and the resolved runner does not report a valid semantic version
- **THEN** the launcher proceeds with the resolved runner without version compatibility checks and warns that reconciliation was forced

#### Scenario: Runner VM sizing is prepared

- **WHEN** the launcher initializes or starts a local deployment
- **THEN** it exposes VM CPU, VM memory, and Exasol Local data disk sizing through the runner start command

#### Scenario: Managed share is prepared

- **WHEN** the launcher initializes a local deployment
- **THEN** it creates a launcher-managed share for guest coordination and SSH key import

#### Scenario: User shares are not exposed

- **WHEN** the user initializes or starts a local deployment
- **THEN** the launcher does not require or expose user-configurable shared folder settings
