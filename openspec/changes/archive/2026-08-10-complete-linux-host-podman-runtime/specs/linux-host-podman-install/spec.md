## ADDED Requirements

### Requirement: Linux host installation reports exact container status
The system SHALL report whether the deployment-specific Nano container is running without matching unrelated containers.

#### Scenario: Deployment container is running
- **WHEN** the exact deployment container exists and is running
- **THEN** installation status is running

#### Scenario: Deployment container is absent or stopped
- **WHEN** the exact deployment container is absent or is not running
- **THEN** installation status is stopped

### Requirement: Podman startup failures include best-effort diagnostics
The system SHALL emit Podman environment, container-list, deployment-container inspection, and deployment-container log diagnostics when image materialization, container recreation, or startup fails, without replacing the original failure.

#### Scenario: Startup fails and diagnostics partly fail
- **WHEN** startup fails and one or more diagnostic commands also fail
- **THEN** the original startup error is returned and all remaining diagnostics are attempted

### Requirement: Nano version checks are configured portably
The system SHALL always tell Nano whether version checks are enabled and, when enabled, SHALL pass the configured service URL, identity, Linux operating-system identity, bounded interval, and bounded retry interval.

#### Scenario: Version checks are disabled
- **WHEN** version checking is disabled
- **THEN** Nano receives the disabled setting and no enabled-only settings

#### Scenario: Version-check interval is absent or outside supported limits
- **WHEN** version checking is enabled with an absent or out-of-range interval
- **THEN** Nano receives the default or nearest supported interval and a retry interval no greater than one day

### Requirement: Configured SLCs are materialized and mounted
The system SHALL reuse available SLC images, pull missing official images, import available custom packages as managed images, mount every available configured image, and atomically publish an availability report.

#### Scenario: Missing official image is available remotely
- **WHEN** a configured official image is not present locally and can be pulled
- **THEN** it is pulled and mounted into Nano at its configured target

#### Scenario: Official image cannot be pulled
- **WHEN** a configured official image is missing and its pull fails
- **THEN** database startup fails with the pull error

#### Scenario: Custom package is missing or invalid
- **WHEN** a configured custom package is missing or cannot be imported
- **THEN** it is reported unavailable and skipped while the database still starts

#### Scenario: Availability report is replaced
- **WHEN** configured SLC materialization completes
- **THEN** the complete new report atomically replaces the previous report without exposing a partial report

### Requirement: Unreferenced managed SLC images are pruned safely
The system SHALL prune only unreferenced tagged images from the exact official SLC repository and labeled custom imports, and SHALL treat equivalent registry prefixes and implicit `latest` tags as the same reference.

#### Scenario: Caller is unaware of SLC state
- **WHEN** the startup configuration contains a nil SLC list
- **THEN** no SLC images are pruned

#### Scenario: Caller declares an authoritative empty SLC set
- **WHEN** the startup configuration contains a non-nil empty SLC list
- **THEN** all otherwise-prunable managed SLC images are considered unreferenced

#### Scenario: Image is unrelated or untagged
- **WHEN** a local image belongs to another repository, lacks the custom-import label, or is untagged
- **THEN** it is not pruned

#### Scenario: Pruning fails
- **WHEN** an eligible image cannot be removed
- **THEN** startup continues and reports the cleanup failure as a warning

### Requirement: Interrupted initial Nano creation is recovered
The system SHALL detect an interrupted initial-create marker, remove the disposable container, quarantine the partial persistent data, recreate an empty data directory, and retry initial creation without destroying the quarantined data.

#### Scenario: Initial creation was interrupted
- **WHEN** the initial-create marker exists at startup
- **THEN** the partial data directory is moved to a timestamped sibling and startup proceeds with an empty data directory

#### Scenario: Stale TLS files exist without database configuration
- **WHEN** no database configuration exists but stale Nano TLS key or certificate files exist
- **THEN** only those stale TLS files are removed before initial creation

#### Scenario: Existing database has TLS files
- **WHEN** database configuration exists
- **THEN** existing TLS files are preserved and first-start parameters are not passed again

### Requirement: Legacy overlay-backed Nano data is migrated safely
The system SHALL migrate a legacy deployment container whose `/exa` is not persistently mounted into deployment-owned storage without removing the source container until a complete staged copy can be atomically installed.

#### Scenario: Persistent destination is populated
- **WHEN** legacy overlay data is detected and the persistent destination contains data
- **THEN** migration is refused without overwriting the destination or removing the legacy container

#### Scenario: Legacy copy fails
- **WHEN** copying `/exa` from the stopped legacy container fails
- **THEN** the legacy container and any recoverable staged state are retained and the error explains how to recover

#### Scenario: Legacy copy succeeds
- **WHEN** the legacy `/exa` copy completes into a sibling staging directory and can be atomically installed
- **THEN** the persistent destination becomes active before the old container is removed

