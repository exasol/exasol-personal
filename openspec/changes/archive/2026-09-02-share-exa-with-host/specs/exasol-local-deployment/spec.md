## MODIFIED Requirements

### Requirement: Launcher-owned local runtime

The system SHALL own host-side Nano data, the macOS VM runtime disk, and the
managed deployment share inside the deployment directory, and SHALL resolve the
Exasol Local VM runner through the resource manager on every use rather than
maintaining a per-deployment copy of it.

#### Scenario: Runner is resolved without a per-deployment copy

- **WHEN** the launcher initializes or starts a local deployment
- **THEN** it resolves the Exasol Local runner through the resource manager and
  invokes it directly from the resolved location, without copying it into the
  deployment directory

#### Scenario: Missing version marker is initialized

- **WHEN** the launcher prepares a local deployment that has no persisted
  runner-version marker, or an invalid one, and the resolved runner reports a
  valid semantic version
- **THEN** it records the resolved runner's version as the deployment's
  persisted marker before invoking the runner

#### Scenario: Compatible runner update is recorded

- **WHEN** the resolved runner is a newer patch or minor version within the
  persisted marker's major version
- **THEN** the launcher updates the persisted marker to the resolved runner's
  version before starting the local deployment

#### Scenario: Release-candidate runner update is recorded

- **WHEN** a `v`-prefixed resolved runner release candidate has greater semantic
  precedence than the persisted marker's release candidate within the same
  major version
- **THEN** the launcher updates the persisted marker to the resolved runner's
  version before starting the local deployment

#### Scenario: Unsafe version relationship proceeds with a warning

- **WHEN** the resolved runner's version differs in major version from the
  persisted marker, or is older than the persisted marker within the same major
  version
- **THEN** the launcher proceeds using the resolved runner, logs a warning
  describing the version relationship, and updates the persisted marker to the
  resolved runner's version

#### Scenario: Version reconciliation is skipped for non-starting lifecycle behavior

- **WHEN** the launcher performs status, stop, or destroy behavior for a local
  deployment
- **THEN** it resolves and invokes the runner without comparing or updating the
  persisted version marker

#### Scenario: Resolved runner version is invalid

- **WHEN** the resolved runner does not report a valid semantic version during
  preparation
- **THEN** the launcher fails before invoking it, unless forced reconciliation
  is enabled

#### Scenario: Internal forced-reconciliation bypass is enabled

- **WHEN** development explicitly enables forced reconciliation and the
  resolved runner does not report a valid semantic version
- **THEN** the launcher proceeds with the resolved runner without version
  compatibility checks and warns that reconciliation was forced

#### Scenario: Runner VM sizing is prepared

- **WHEN** the launcher initializes or starts a local deployment
- **THEN** it exposes VM CPU, VM memory, and VM runtime-disk sizing through the
  runner start command

#### Scenario: Managed share is prepared

- **WHEN** the launcher initializes a local deployment
- **THEN** it creates a launcher-managed share for guest coordination, SSH key
  import, and host-visible Nano data

#### Scenario: User shares are not exposed

- **WHEN** the user initializes or starts a local deployment
- **THEN** the launcher does not require or expose user-configurable shared
  folder settings

### Requirement: Local lifecycle commands

The system SHALL support standard lifecycle commands for local deployments and
preserve host-backed Nano data until the deployment is destroyed.

#### Scenario: Stop local deployment

- **WHEN** a macOS local deployment is running and the user runs `exasol stop`
- **THEN** the launcher removes the disposable Nano container, stops the VM,
  preserves host-backed Nano data, and records the deployment as stopped

#### Scenario: Start local deployment

- **WHEN** a local deployment is stopped and the user runs `exasol start`
- **THEN** the launcher starts its execution environment with the existing
  host-backed Nano data, starts the shared Podman installation, waits up to a
  bounded timeout until the database accepts connections, refreshes connection
  artifacts, and records the deployment as running

#### Scenario: Start local deployment times out

- **WHEN** a local deployment is stopped, the user runs `exasol start`, and the
  database does not become ready within the bounded timeout
- **THEN** the launcher fails the command rather than waiting indefinitely

#### Scenario: Local execution environment stops unexpectedly

- **WHEN** a local execution environment stops without a graceful Nano shutdown
  and the deployment is started again
- **THEN** the launcher recovers the runtime with the same host-backed Nano data

#### Scenario: Destroy local deployment

- **WHEN** a user runs `exasol destroy` for a local deployment
- **THEN** the launcher removes the disposable container, stops its execution
  environment if needed, deletes deployment-owned host-backed Nano data, any
  macOS migration backup, and connection artifacts, and records the deployment
  as initialized

### Requirement: Local configuration is platform-specific

The system SHALL expose and validate VM CPU, memory, and runtime-disk sizing
only for the macOS VM runtime, while direct-host platforms SHALL ignore
persisted VM sizing and use host-managed resources.

#### Scenario: Linux local configuration contains VM sizing

- **WHEN** a Linux deployment contains shared VM CPU, memory, or data-size
  values
- **THEN** local validation and startup ignore those values and do not advertise
  them as effective Linux settings

#### Scenario: Windows local configuration contains VM sizing

- **WHEN** a Windows deployment contains shared VM CPU, memory, or data-size
  values
- **THEN** local validation and startup ignore those values and do not advertise
  them as effective Windows settings

#### Scenario: macOS local configuration contains invalid VM sizing

- **WHEN** a macOS Apple Silicon deployment contains invalid VM sizing
- **THEN** validation fails as before

#### Scenario: macOS data-size configuration is inspected

- **WHEN** a user inspects local configuration on macOS
- **THEN** the data-size setting describes VM runtime-disk capacity for Podman
  images and runtime state


## ADDED Requirements

### Requirement: Local Nano data is host-visible

The system SHALL expose the complete persistent Nano `/exa` tree at
`local/runtime/exa` in the deployment directory on every supported local
platform.

#### Scenario: Fresh local deployment starts

- **WHEN** a user installs a fresh local deployment on Linux, macOS, or Windows
- **THEN** Nano stores its persistent `/exa` content in
  `local/runtime/exa`

#### Scenario: Host creates a runtime file

- **WHEN** a file is created below `local/runtime/exa` while the local runtime
  is available
- **THEN** the file is visible at the corresponding path below Nano's `/exa`

#### Scenario: Nano creates a runtime file

- **WHEN** Nano creates a file below `/exa`
- **THEN** the file is visible at the corresponding path below
  `local/runtime/exa`

#### Scenario: Nano runs an SLC

- **WHEN** Nano initializes or runs an SLC in a local deployment
- **THEN** persistent SLC files remain visible below
  `local/runtime/exa/slc`

#### Scenario: Existing direct-host deployment starts

- **WHEN** an existing Linux or Windows deployment has managed host-side Nano
  data
- **THEN** the launcher continues using that data at `local/runtime/exa`
