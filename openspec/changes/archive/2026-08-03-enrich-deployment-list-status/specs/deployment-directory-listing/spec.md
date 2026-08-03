## REMOVED Requirements

### Requirement: Listing SHALL report initialization status and preset identity
**Reason**: Superseded by a requirement to report the full deployment lifecycle status instead of a binary initialized/not_initialized check. See the ADDED "Listing SHALL report deployment status and preset identity" requirement below.
**Migration**: Consumers parsing `status` must accept the full lifecycle vocabulary (`not_initialized`, `initialized`, `operation_in_progress`, `interrupted`, `deployment_failed`, `stopped`, `running`) instead of assuming only `initialized`/`not_initialized`. Consumers that only checked `status == "not_initialized"` need no change; any value other than `not_initialized` still indicates preset identity is present.

### Requirement: Listing SHALL indicate the currently active deployment directory
**Reason**: The `active` field indicated CWD-based directory-selection precedence (which directory a flagless command would target), not deployment lifecycle state. This was judged unintuitive and not useful in a command whose purpose is to list every deployment directory at once.
**Migration**: No replacement field is added. A user who needs to know which directory a flagless command would currently target should run that command directly (e.g. `exasol status`), which reports the resolved deployment directory.

## MODIFIED Requirements

### Requirement: Listing SHALL support JSON output
`exasol deployments list` SHALL support a `--json` flag that emits the same information as structured JSON instead of human-readable text.

#### Scenario: JSON output includes all listed fields
- **WHEN** a user runs `exasol deployments list --json`
- **THEN** stdout is valid JSON
- **AND** each entry includes name, path, status, and preset identity when the status is not `not_initialized`

## ADDED Requirements

### Requirement: Listing SHALL report deployment status and preset identity
For each listed deployment directory, `exasol deployments list` SHALL report its deployment lifecycle status using the same status vocabulary as `exasol status` (`not_initialized`, `initialized`, `operation_in_progress`, `interrupted`, `deployment_failed`, `stopped`, `running`), and when that status is not `not_initialized`, its infrastructure and installation preset identity.

#### Scenario: Uninitialized deployment is reported without failing the listing
- **WHEN** a user runs `exasol deployments list`
- **AND** a listed deployment directory exists but has never been initialized
- **THEN** that entry reports status `not_initialized`
- **AND** the command still succeeds and lists the remaining entries

#### Scenario: An unreadable or corrupt deployment state is reported as not initialized
- **WHEN** a user runs `exasol deployments list`
- **AND** a listed deployment directory's state file exists but cannot be read or parsed
- **THEN** that entry reports status `not_initialized`
- **AND** the command still succeeds and lists the remaining entries

#### Scenario: Deployed and running deployment reports its actual lifecycle status
- **WHEN** a user runs `exasol deployments list`
- **AND** a listed deployment directory has been deployed and is currently running
- **THEN** that entry reports status `running`
- **AND** that entry reports the infrastructure and installation preset identity

#### Scenario: Stopped deployment reports its actual lifecycle status
- **WHEN** a user runs `exasol deployments list`
- **AND** a listed deployment directory has been deployed and subsequently stopped
- **THEN** that entry reports status `stopped`
- **AND** that entry reports the infrastructure and installation preset identity

#### Scenario: Deployment with an in-progress, interrupted, or failed operation reports that status
- **WHEN** a user runs `exasol deployments list`
- **AND** a listed deployment directory has a pending, interrupted, or failed deploy/destroy operation
- **THEN** that entry reports status `operation_in_progress`, `interrupted`, or `deployment_failed` respectively
- **AND** that entry reports the infrastructure and installation preset identity

#### Scenario: Listing a deployment directory does not modify it
- **WHEN** a user runs `exasol deployments list`
- **AND** a listed deployment directory's status is not `not_initialized` but its preset identity is not yet persisted in its state file
- **THEN** that entry reports a derived preset identity
- **AND** the deployment directory's state file is not modified as a result of running `exasol deployments list`
