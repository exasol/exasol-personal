## Purpose

Defines the mechanics shared by every legacy-to-new-location path migration: automatic execution, reporting what changed, quitting on failure, and atomic moves.

## ADDED Requirements

### Requirement: Migration SHALL run automatically
Startup path migration SHALL apply its changes automatically as part of CLI startup, consistent with every other automatic migration already in the launcher.

#### Scenario: Migration applies changes automatically
- **WHEN** startup migration has changes to make
- **THEN** the CLI makes those changes immediately, the same way whether or not stdin is interactive

### Requirement: Migration SHALL report what it changed
Startup path migration SHALL emit a clear human-facing message identifying what it moved or deleted and where, before any output from the requested command.

#### Scenario: Migration reports its changes
- **WHEN** startup migration makes any change (moving a deployment, history file, or cache config; deleting the legacy cache)
- **THEN** the CLI emits a message on standard error naming what changed and its new location, before the requested command produces any output

#### Scenario: Migration stays quiet when there is nothing to do
- **WHEN** startup migration finds nothing to migrate
- **THEN** the CLI proceeds straight to the requested command's own output

### Requirement: The invocation SHALL quit before command dispatch on migration failure
When startup migration fails, the CLI SHALL exit with a clear error before dispatching to the requested command, rather than proceeding against a mixed legacy/new-location state.

#### Scenario: Failed migration blocks the requested command
- **WHEN** startup migration fails partway (an I/O error, or a refused naming collision)
- **THEN** the CLI exits with a non-zero status describing the failure before running the requested command

#### Scenario: Successful migration proceeds to the requested command
- **WHEN** startup migration completes successfully (including when there was nothing to migrate)
- **THEN** the CLI proceeds to dispatch the requested command normally

### Requirement: Migration SHALL run at most once per completed location
Once migration for a given location (managed deployments, SQL history, or cache configuration) has completed, later CLI invocations SHALL treat that location as already migrated.

#### Scenario: A completed location is skipped on later invocations
- **WHEN** startup migration has already completed for a given location
- **THEN** subsequent CLI invocations skip migrating that location again

### Requirement: Each migrated item SHALL be complete at exactly one location
Startup path migration SHALL move each deployment, history file, and cache config with a single atomic rename, so an interrupted migration leaves every item complete at either its legacy or its new location. When an item cannot be renamed to its new location, migration SHALL fail with an error naming both paths and leave the legacy item unchanged.

#### Scenario: Interrupted migration resumes on the next invocation
- **WHEN** a previous migration attempt was interrupted after moving some items
- **THEN** the next invocation migrates the items still at their legacy location

#### Scenario: Unmovable item stops migration
- **WHEN** a legacy item cannot be renamed to its new location, for example because the two locations are on different filesystems
- **THEN** migration fails with an error naming both paths
- **AND** the legacy item is unchanged

### Requirement: Migration SHALL wait until no legacy deployment is in use
Before moving anything, startup path migration SHALL check every legacy deployment. While a legacy local deployment runs or another launcher process holds a legacy deployment's lock, every command except `stop` and `diag unlock` SHALL exit with an error listing the command that clears each blocking deployment, and all legacy items SHALL stay at their legacy locations.

#### Scenario: Running local deployment blocks migration
- **WHEN** a local deployment at the legacy location is running
- **THEN** a command other than `stop` or `diag unlock` exits with an error listing `exasol stop --deployment-dir <legacy path>`
- **AND** all legacy items stay at their legacy locations

#### Scenario: Locked legacy deployment blocks migration
- **WHEN** another launcher process holds the lock of a deployment at the legacy location
- **THEN** a command other than `stop` or `diag unlock` exits with an error naming `exasol diag unlock --deployment-dir <legacy path>`
- **AND** all legacy items stay at their legacy locations

#### Scenario: Stop and unlock run while migration is blocked
- **WHEN** migration is blocked and the user runs `exasol stop --deployment-dir <legacy path>` or `exasol diag unlock --deployment-dir <legacy path>`
- **THEN** the CLI runs that command against the legacy location without migrating

### Requirement: Migration SHALL remove the legacy directories it empties
After a successful migration, the CLI SHALL remove each empty legacy launcher directory, up to and including the `.exasol` directory that contains it, and SHALL keep every legacy directory that still holds content.

#### Scenario: Emptied legacy directories are removed
- **WHEN** migration moves every item out of `~/.exasol/personal` and deletes the legacy cache
- **THEN** `~/.exasol/personal` and the legacy cache's `.exasol` directory no longer exist

#### Scenario: Legacy directory with remaining content is kept
- **WHEN** a legacy directory still holds content after a successful migration
- **THEN** that directory and its content stay in place
