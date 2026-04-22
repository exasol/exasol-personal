# local-runtime-assets Specification

## ADDED Requirements

### Requirement: Versioned local runtime payload distribution

The system SHALL obtain Linux ExaNano `.run` payloads for local mode as versioned artifacts from a product-owned HTTP location.

#### Scenario: Resolve payload metadata for local deployment

- GIVEN the user is preparing a local deployment
- WHEN the launcher resolves the required ExaNano payload
- THEN it uses product-owned payload metadata that identifies a versioned Linux `.run` artifact for the selected guest architecture

### Requirement: Launcher-owned guest execution

The system SHALL execute the selected Linux `.run` payload inside the launcher-owned VM instead of delegating local execution to a separate native macOS ExaNano runtime.

#### Scenario: Start local database inside the guest

- GIVEN the launcher has selected a Linux `.run` payload for a local deployment
- AND the launcher has booted the local VM
- WHEN guest bootstrap starts the database
- THEN it invokes the selected `.run` payload inside the guest
- AND the launcher remains the owner of the host-side virtualization lifecycle

### Requirement: Payload verification and caching

The system SHALL verify and cache downloaded local runtime payloads before using them.

#### Scenario: Download and verify payload

- GIVEN the required payload is not present in the local cache
- WHEN the launcher downloads the payload
- THEN it verifies the payload against expected integrity metadata
- AND stores the verified payload in an Exasol-owned cache location

#### Scenario: Reuse cached payload

- GIVEN the required payload is already present in the local cache
- WHEN the launcher prepares the local runtime
- THEN it reuses the cached payload instead of downloading it again

#### Scenario: Reject invalid payload

- GIVEN a downloaded payload fails integrity verification
- WHEN the launcher validates the payload
- THEN it refuses to use the payload
- AND reports a clear verification error

### Requirement: Deployment records selected payload identity

The system SHALL persist the selected payload identity into deployment-owned local runtime state.

#### Scenario: Persist payload version and architecture

- GIVEN the launcher has selected a payload for a local deployment
- WHEN it writes deployment-owned local runtime state
- THEN that state records the payload version
- AND records the selected guest architecture or equivalent payload identity
