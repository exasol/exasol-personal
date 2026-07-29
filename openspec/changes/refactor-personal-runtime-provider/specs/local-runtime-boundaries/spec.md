## MODIFIED Requirements

### Requirement: Local runtime implementation is isolated

The system SHALL keep platform runtime mechanics behind runtime adapters while Personal
owns Nano image selection, Podman manifests, persistence, SLC mounts, readiness,
workload lifecycle, and migration policy. local-vm v2 SHALL be consumed only as a
generic macOS VM provider.

#### Scenario: macOS local lifecycle is delegated

- **WHEN** Personal starts a macOS local deployment
- **THEN** it supplies a generic VM configuration and a caller-owned boot hook
- **AND** local-vm does not interpret Nano or Exasol-specific behavior

#### Scenario: Windows local lifecycle is delegated

- **WHEN** Personal starts a Windows local deployment
- **THEN** it reconciles the same Personal-owned workload through the direct Podman adapter
- **AND** does not introduce a Windows implementation into local-vm

#### Scenario: Deployment workflow remains the artifact owner

- **WHEN** an adapter reports live endpoint state after deploy or start
- **THEN** the local backend maps that state into standard launcher deployment artifacts
- **AND** does not persist adapter identities, manifests, or Podman object IDs
