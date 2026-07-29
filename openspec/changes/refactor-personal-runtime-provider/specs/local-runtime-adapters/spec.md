## ADDED Requirements

### Requirement: Personal reconstructs a single runtime model

Personal SHALL reconstruct the local workload from deployment configuration for every
command and reconcile it through the platform adapter without persisting adapter IDs,
container IDs, manifests, or image archives.

#### Scenario: Read-only status after generated files are deleted

- **WHEN** generated runtime files are absent and status is requested
- **THEN** Personal derives the workload without modifying persistent state
- **AND** discovers live state through the selected adapter

### Requirement: macOS executes only the pinned v2 provider

Personal SHALL stage and invoke only the local-vm v2 artifact pinned by the launcher
build and SHALL reject version or schema mismatches without falling back to v1.

#### Scenario: A different provider version is staged

- **WHEN** `version --json` reports a version other than the release-pinned version
- **THEN** the lifecycle operation fails before VM reconciliation
