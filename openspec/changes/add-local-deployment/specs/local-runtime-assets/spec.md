# local-runtime-assets Specification

## ADDED Requirements

### Requirement: Build-time embedded local runtime payload bundle

The system SHALL include a build-time embedded local runtime payload bundle for supported local launcher builds.

#### Scenario: Launcher contains the local payload baseline

- GIVEN the launcher is built for a supported local deployment target
- WHEN the local runtime payload is packaged at build time
- THEN the launcher binary contains the Linux ExaNano `.run` payload for the selected guest architecture
- AND contains the guest kernel and initrd required to boot the launcher-owned VM
- AND contains metadata required to identify and verify those embedded artifacts

### Requirement: Launcher-owned guest execution

The system SHALL execute the selected Linux `.run` payload inside the launcher-owned VM instead of delegating local execution to a separate native macOS ExaNano runtime.

#### Scenario: Start local database inside the guest

- GIVEN the launcher has selected a Linux `.run` payload for a local deployment
- AND the launcher has booted the local VM
- WHEN guest bootstrap starts the database
- THEN it invokes the selected `.run` payload inside the guest
- AND the launcher remains the owner of the host-side virtualization lifecycle

### Requirement: Embedded payload cache seeding and verification

The system SHALL verify and cache embedded local runtime payload artifacts before using them.

#### Scenario: Seed cache from embedded payload bundle

- GIVEN the required embedded payload artifacts are not present in the local cache
- WHEN the launcher prepares the local runtime
- THEN it extracts the embedded payload bundle into an Exasol-owned cache location
- AND verifies the extracted `.run`, kernel, and initrd against the embedded metadata before use

#### Scenario: Reuse cached embedded payload

- GIVEN the required embedded payload artifacts are already present in the local cache
- WHEN the launcher prepares the local runtime
- THEN it reuses the cached payload artifacts instead of re-extracting them

#### Scenario: Reject invalid embedded payload extraction

- GIVEN an extracted embedded payload artifact fails integrity verification
- WHEN the launcher validates the embedded payload cache entry
- THEN it refuses to use that cache entry
- AND reports a clear verification error

### Requirement: Deployment records selected payload identity

The system SHALL persist the selected payload identity into deployment-owned local runtime state.

#### Scenario: Persist payload version and architecture

- GIVEN the launcher has selected a payload for a local deployment
- WHEN it writes deployment-owned local runtime state
- THEN that state records the payload version
- AND records the selected guest architecture or equivalent payload identity

### Requirement: Remote payload downloading is disabled for the embedded baseline

The system SHALL not require runtime HTTP payload downloading for the current embedded local payload model.

#### Scenario: Start local mode without remote payload fetch

- GIVEN the launcher contains the embedded local runtime payload baseline
- WHEN the user starts a new local deployment
- THEN the launcher resolves the payload from the embedded bundle and local cache only
- AND does not fetch the local runtime payload from a remote HTTP endpoint
