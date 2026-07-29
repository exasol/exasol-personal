## ADDED Requirements

### Requirement: Legacy deployments migrate without v1 execution

Personal SHALL preserve legacy settings and data while executing only launcher v2 and
its pinned provider. Migration SHALL run only from the v2 boot hook and SHALL not add a
format version or checkpoint to launcher state.

#### Scenario: A stopped legacy deployment starts under v2

- **WHEN** pre-contract VM state contains legacy `/exa` data
- **THEN** v2 preserves its provider-owned `/var` disk while refreshing provider assets
- **AND** the Personal boot hook copies and verifies the legacy data in a sibling staging
  directory before atomically installing the virtiofs target

#### Scenario: Migration fails

- **WHEN** stopping the legacy container, copying data, verification, or workload apply fails
- **THEN** provider start fails with the hook error
- **AND** the legacy source and any staging data are retained

#### Scenario: Migration succeeds

- **WHEN** the boot hook atomically installs `<deployment>/local/data/exa`
- **THEN** that directory is the migration completion marker
- **AND** subsequent starts skip migration without changing launcher state or the
  deployment compatibility marker
