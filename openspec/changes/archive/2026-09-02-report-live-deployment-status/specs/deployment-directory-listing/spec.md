## MODIFIED Requirements

### Requirement: Listing SHALL report deployment status and preset identity
For each listed deployment directory, `exasol deployments list` SHALL report the same current status as `exasol status`, and when that status is not `not_initialized`, its infrastructure and installation preset identity. The command SHALL resolve deployment statuses concurrently under one five-second bound.

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

#### Scenario: Ready deployment reports its current status
- **WHEN** a user runs `exasol deployments list`
- **AND** a listed deployment's database is running and ready
- **THEN** that entry reports status `database_ready`
- **AND** that entry reports the infrastructure and installation preset identity

#### Scenario: Unreachable deployment reports its current status
- **WHEN** a user runs `exasol deployments list`
- **AND** a listed deployment's persisted state is running but its database is unreachable
- **THEN** that entry reports status `database_connection_failed`
- **AND** that entry reports the infrastructure and installation preset identity

#### Scenario: Stopped deployment reports its current status
- **WHEN** a user runs `exasol deployments list`
- **AND** a listed deployment has been deployed and subsequently stopped
- **THEN** that entry reports status `stopped`
- **AND** that entry reports the infrastructure and installation preset identity

#### Scenario: Deployment with an in-progress, interrupted, or failed operation reports that status
- **WHEN** a user runs `exasol deployments list`
- **AND** a listed deployment directory has a pending, interrupted, or failed deploy/destroy operation
- **THEN** that entry reports status `operation_in_progress`, `interrupted`, or `deployment_failed` respectively
- **AND** that entry reports the infrastructure and installation preset identity

#### Scenario: Multiple slow status checks share one bound
- **WHEN** multiple listed deployments do not complete their status checks within five seconds
- **THEN** the listing stops waiting for all of them after five seconds
- **AND** the command still succeeds and lists every deployment directory

#### Scenario: Listing a deployment directory does not modify it
- **WHEN** a user runs `exasol deployments list`
- **AND** a listed deployment directory's status is not `not_initialized` but its preset identity is not yet persisted in its state file
- **THEN** that entry reports a derived preset identity
- **AND** the deployment directory's state file is not modified as a result of running `exasol deployments list`
