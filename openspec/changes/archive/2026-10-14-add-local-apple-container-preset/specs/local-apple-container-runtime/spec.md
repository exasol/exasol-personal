## ADDED Requirements

### Requirement: Local Apple Container deployment preset
The system SHALL provide `local-apple-container` as a local deployment preset that runs Nano through Apple's `container` CLI directly on the host, selectable independently of the `local` preset's VM-backed Podman runtime.

#### Scenario: Install local-apple-container deployment
- **WHEN** a user runs `exasol install local-apple-container` in an empty deployment directory on macOS Apple Silicon
- **THEN** the launcher prepares the `container` CLI and its system service, initializes the deployment directory, starts Nano through `container`, waits up to a bounded timeout until the database accepts connections, and records the deployment as running

#### Scenario: Install local-apple-container deployment times out
- **WHEN** a user runs `exasol install local-apple-container` and the database does not become ready within the bounded timeout
- **THEN** the launcher fails the command rather than waiting indefinitely

#### Scenario: Reject unsupported platform
- **WHEN** a user runs `exasol install local-apple-container` on an operating system or architecture other than macOS Apple Silicon
- **THEN** the launcher fails before starting a runtime and states that the preset requires macOS Apple Silicon

### Requirement: Local Apple Container environment is prepared with explicit approval
The system SHALL ensure the `container` CLI and its system service are available before deploying Nano, and SHALL require approval before installing the `container` CLI through Homebrew when available or through Apple's signed installer package otherwise, without requiring Homebrew to be present.

#### Scenario: `container` is already available
- **WHEN** the `container` CLI can be resolved and its system service is already running
- **THEN** preparation continues without requesting approval or installing anything

#### Scenario: `container` system service is not running
- **WHEN** the CLI is present but its system service is not running
- **THEN** the launcher starts the system service without requesting approval

#### Scenario: CLI installation is approved through Homebrew
- **WHEN** the CLI cannot be resolved, Homebrew is available, and the displayed Homebrew install command is approved
- **THEN** the launcher installs `container` through Homebrew and verifies it before continuing

#### Scenario: CLI installation is approved through the signed installer package
- **WHEN** the CLI cannot be resolved, Homebrew is not available, and the displayed installer command is approved
- **THEN** the launcher downloads the pinned signed installer package, verifies it against its recorded checksum, installs it, and verifies the CLI before continuing

#### Scenario: CLI installation is declined
- **WHEN** the CLI is unavailable and the host change is declined
- **THEN** preparation fails without installing anything

#### Scenario: Neither installation mechanism is available
- **WHEN** the CLI is unavailable, Homebrew is not available, and the signed installer package cannot be downloaded or fails checksum verification
- **THEN** preparation fails without requesting approval and explains how to install `container` manually

### Requirement: Local Apple Container deployment reuses shared local deployment behavior
The system SHALL reuse the generated `sys` password, host-visible `/exa` data layout, deterministic port allocation and bind-conflict handling, and bounded readiness wait already specified for local deployments.

#### Scenario: Standard local behavior applies
- **WHEN** a user installs, connects to, stops, starts, or destroys a `local-apple-container` deployment
- **THEN** it uses the same generated-password, data-visibility, port-allocation, and readiness-wait contract as other local deployments

### Requirement: Local Apple Container deployment does not support script language containers
The system SHALL reject script language container installation for `local-apple-container` deployments with an explicit, scoped error, rather than attempting an unsupported container mount.

#### Scenario: Script language container installation is requested
- **WHEN** a user attempts to install a script language container on a `local-apple-container` deployment
- **THEN** the command fails before attempting any container mount and states that script language containers are not yet supported on this preset

### Requirement: Local Apple Container deployment does not restart Nano automatically
The system SHALL NOT automatically restart a Nano container that stops unexpectedly on a `local-apple-container` deployment; recovery SHALL require `exasol start`.

#### Scenario: Nano stops unexpectedly
- **WHEN** the Nano container for a `local-apple-container` deployment stops outside a launcher command
- **THEN** the deployment is reported as stopped or unhealthy until the user runs `exasol start`, rather than being restarted automatically
