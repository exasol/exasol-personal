## MODIFIED Requirements

### Requirement: Local diagnostics command

The system SHALL provide a read-only local diagnostics command that combines
configuration, capabilities, provider or Podman state, forwards, and SQL readiness
without staging artifacts or changing launcher state.

#### Scenario: Diagnostics run for a stopped deployment

- **WHEN** diagnostics are requested while the workload is stopped
- **THEN** the response retains available configuration and provider details
- **AND** the launcher state file remains byte-for-byte unchanged

#### Scenario: Diagnostics report platform support

- **WHEN** diagnostics run on an unsupported operating system or architecture
- **THEN** the response reports that the local preset is unsupported
- **AND** does not require or mutate a running deployment

#### Scenario: Diagnostics report a healthy runtime

- **WHEN** diagnostics run for a healthy local deployment
- **THEN** the response combines platform capabilities, live provider or Podman state,
  forwarding health, workload state, and SQL readiness

#### Scenario: Runtime state conflicts with workflow state

- **WHEN** live adapter state conflicts with the recorded deployment workflow state
- **THEN** diagnostics report the inconsistency as a warning alongside all other
  state that can still be discovered
