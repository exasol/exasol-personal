## MODIFIED Requirements

### Requirement: Exasol Local deployment preset

The system SHALL provide a local deployment option for macOS Apple Silicon using a
dedicated local-vm v2 provider and for Windows amd64 using direct Podman on WSL2.

#### Scenario: Install on macOS Apple Silicon

- **WHEN** a user installs a local deployment on supported macOS Apple Silicon
- **THEN** Personal starts the pinned local-vm v2 provider and generated Nano workload
- **AND** waits for SQL readiness within the existing bounded timeout

#### Scenario: Install on Windows WSL2

- **WHEN** a user installs a local deployment on supported Windows amd64 with WSL2
- **THEN** Personal reconciles the generated Nano workload through the current or
  default Podman machine
- **AND** waits for SQL readiness within the existing bounded timeout

#### Scenario: Reject an unsupported local platform

- **WHEN** a user installs a local deployment on another operating system,
  architecture, or Windows provider
- **THEN** Personal fails before starting runtime resources and explains the supported
  platform requirements

#### Scenario: Install local deployment times out

- **WHEN** the generated local workload does not become SQL-ready within the bounded timeout
- **THEN** Personal fails the command rather than waiting indefinitely

### Requirement: Launcher-owned local runtime

The system SHALL derive a disposable local runtime from the deployment manifest and
launcher state, stage only the provider pinned by the current launcher, and preserve
Personal-owned data independently of provider state.

#### Scenario: The v2 provider is staged

- **WHEN** Personal prepares a macOS local deployment
- **THEN** it atomically stages the embedded local-vm v2 artifact at the established
  deployment staging location
- **AND** validates its exact version and config, hook, and state schemas

#### Scenario: An incompatible provider is present

- **WHEN** the staged provider reports a different version or schema contract
- **THEN** Personal fails before VM reconciliation
- **AND** does not invoke or fall back to a v1 runner

#### Scenario: Runtime artifacts are missing

- **WHEN** a mutating lifecycle operation runs after generated configuration, hook, or
  Kube files have been deleted
- **THEN** Personal reconstructs them from deployment configuration and launcher state

#### Scenario: macOS VM resources are prepared

- **WHEN** Personal prepares a macOS local deployment
- **THEN** it includes the configured CPU and memory in the local-vm configuration

#### Scenario: Windows machine settings remain user-owned

- **WHEN** Personal prepares a Windows local deployment
- **THEN** it does not expose or change CPU, memory, storage size, or Podman machine mode

### Requirement: Local deployment artifacts

The system SHALL write standard endpoint-based launcher artifacts for local deployments
so information, connection, and status commands do not depend on adapter-specific state.

#### Scenario: Connection artifacts are written after startup

- **WHEN** the local workload becomes SQL-ready
- **THEN** Personal writes deployment, secret, and connection instruction artifacts
  with the resolved loopback database endpoint

#### Scenario: Local credentials are available

- **WHEN** Personal writes secrets for a local deployment
- **THEN** the credentials identify database user `sys` with password `exasol`

#### Scenario: Forwarded ports are refreshed

- **WHEN** a local deployment starts after being stopped
- **THEN** Personal refreshes connection artifacts from live adapter endpoints

#### Scenario: Adapter details remain transient

- **WHEN** Personal writes launcher state or deployment artifacts
- **THEN** it does not serialize adapter IDs, Podman object IDs, image archives, or
  generated manifests

### Requirement: Local lifecycle commands

The system SHALL implement local lifecycle commands through the selected v2 runtime
adapter while preserving Personal-owned data until explicit deployment destruction.

#### Scenario: Stop a macOS local deployment

- **WHEN** a user stops a running macOS local deployment
- **THEN** Personal takes down the Nano workload before stopping its dedicated VM

#### Scenario: Stop a Windows local deployment

- **WHEN** a user stops a running Windows local deployment
- **THEN** Personal takes down only that deployment's namespaced Podman workload
- **AND** leaves the user's Podman machine running

#### Scenario: Start a stopped local deployment

- **WHEN** a user starts a stopped local deployment
- **THEN** Personal regenerates disposable assets, reconciles the workload, waits for
  SQL readiness, and refreshes connection artifacts

#### Scenario: Replace runtime state

- **WHEN** the provider, Podman workload, or generated runtime artifacts are replaced
- **THEN** the Personal-owned host data directory remains unchanged

#### Scenario: Destroy a local deployment

- **WHEN** a user explicitly destroys a local deployment
- **THEN** Personal destroys only that deployment's runtime resources and then removes
  its exact Personal-owned data directory

### Requirement: Local shell access

The system SHALL expose VM and container shells only where the selected runtime adapter
declares those capabilities.

#### Scenario: macOS host shell

- **WHEN** a user opens the host shell for a running macOS deployment
- **THEN** Personal opens an interactive SSH session through local-vm's loopback endpoint

#### Scenario: macOS container shell

- **WHEN** a user opens the container shell for a running macOS deployment
- **THEN** Personal opens an interactive shell in the deployment-scoped Nano container

#### Scenario: Windows shell request

- **WHEN** a user requests a host or container shell for a Windows local deployment
- **THEN** Personal rejects the operation as unsupported without mutating the workload
