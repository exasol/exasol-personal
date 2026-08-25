## ADDED Requirements

### Requirement: Windows default Podman machine is ready
The system SHALL use the default Podman machine regardless of whether it operates rootless or rootful, SHALL initialize it using Podman's default mode when absent, and SHALL start it when stopped.

#### Scenario: Default machine is absent
- **WHEN** Podman is available but its default machine does not exist
- **THEN** the launcher initializes that machine with a 40 GB disk using Podman's default mode and starts it

#### Scenario: Default machine is stopped
- **WHEN** the default machine exists but is not running
- **THEN** the launcher starts it without changing its rootful or rootless mode

#### Scenario: Default machine is already running
- **WHEN** the default machine is already running in either rootful or rootless mode
- **THEN** preparation makes no changes to the machine

## MODIFIED Requirements

### Requirement: Windows host preparation is retry-safe
The system SHALL make repeated successful preparation calls safe and SHALL preserve actionable errors for partial prerequisite failures.

#### Scenario: Prepared host is checked again
- **WHEN** Podman and the default machine are already ready
- **THEN** preparation performs only readiness checks and makes no host changes

#### Scenario: A prerequisite command fails
- **WHEN** Windows Package Manager or a Podman-machine command fails
- **THEN** the launcher returns the causal command error and a later deploy or start can retry preparation

## REMOVED Requirements

### Requirement: Windows default Podman machine is ready and rootful
**Reason**: Explicit IPv4 port binding makes Windows local deployments work with Podman's default rootless machine, so enforcing rootful mode is unnecessary and disrupts a shared host resource.

**Migration**: No user action is required; existing rootless and rootful default machines remain in their current mode and are reused.
