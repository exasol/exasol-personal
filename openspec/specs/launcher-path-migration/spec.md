# launcher-path-migration Specification

## Purpose
Defines the mechanics shared by every legacy-to-new-location path migration: automatic execution, reporting what changed, warning on failure, and atomic moves.
## Requirements
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

### Requirement: The requested command SHALL run after a migration warning
When startup migration cannot move or delete a legacy item, the CLI SHALL emit a warning on standard error naming the item and the cause, continue with the remaining items, and then dispatch the requested command.

#### Scenario: Failed migration step warns and the command runs
- **WHEN** startup migration fails to move or delete a legacy item, for example because of an I/O error
- **THEN** the CLI emits a warning on standard error naming the item and the cause
- **AND** the CLI migrates the remaining legacy items
- **AND** the CLI runs the requested command

### Requirement: Migration SHALL run on every invocation until every legacy item is moved
Every CLI invocation SHALL migrate the legacy items that are still at their legacy location.

#### Scenario: Kept item moves on a later invocation
- **WHEN** an earlier invocation kept a legacy deployment at its legacy location
- **AND** nothing keeps it there any longer
- **THEN** the next invocation moves it to its new location

### Requirement: Each migrated item SHALL be complete at exactly one location
Startup path migration SHALL move each deployment, history file, and cache config with a single atomic rename, so an interrupted migration leaves every item complete at either its legacy or its new location. When an item cannot be renamed to its new location, migration SHALL warn naming both paths and leave the legacy item unchanged.

#### Scenario: Interrupted migration resumes on the next invocation
- **WHEN** a previous migration attempt was interrupted after moving some items
- **THEN** the next invocation migrates the items still at their legacy location

#### Scenario: Unmovable item is kept with a warning
- **WHEN** a legacy item cannot be renamed to its new location
- **THEN** migration emits a warning naming both paths
- **AND** the legacy item is unchanged

#### Scenario: Item on another filesystem is left for the user to move
- **WHEN** a legacy item and its new location are on different filesystems
- **THEN** migration emits a warning naming both paths and asking the user to move the item manually
- **AND** the legacy item is unchanged

### Requirement: Concurrent invocations SHALL each complete migration
When several CLI invocations start migration at the same time, each SHALL proceed to its command once every item is at its new location.

#### Scenario: Two invocations migrate at the same time
- **WHEN** two CLI invocations start migration of the same legacy items at the same time
- **THEN** both proceed without a migration warning
- **AND** every legacy item is at its new location

### Requirement: Migration SHALL keep a legacy deployment in use at its legacy location
Startup path migration SHALL keep a legacy deployment at its legacy location, with a warning that names the command to resolve it, while a local deployment there runs, while its running state cannot be determined, or while another launcher process holds its lock. Migration SHALL move the other legacy items.

#### Scenario: Running local deployment is kept
- **WHEN** a local deployment at the legacy location is running
- **THEN** the CLI warns that it kept the deployment at `<legacy path>` and names `exasol stop --deployment-dir <legacy path>`
- **AND** the deployment stays at its legacy location
- **AND** the other legacy items move to their new locations

#### Scenario: Deployment with unknown running state is kept
- **WHEN** the CLI cannot determine whether a local deployment at the legacy location is running
- **THEN** the CLI warns that it kept the deployment at `<legacy path>` and names the cause
- **AND** the deployment stays at its legacy location

#### Scenario: Locked legacy deployment is kept
- **WHEN** another launcher process holds the lock of a deployment at the legacy location
- **THEN** the CLI warns that it kept the deployment at `<legacy path>`, asks the user to close every `exasol` shell or other launcher command using it, and names `exasol diag unlock --deployment-dir <legacy path>`
- **AND** the deployment stays at its legacy location

### Requirement: Migration SHALL remove the legacy directories it empties
After migration, the CLI SHALL remove each empty legacy launcher directory, up to and including the `.exasol` directory that contains it, and SHALL keep every legacy directory that still holds content or contains the working directory.

#### Scenario: Emptied legacy directories are removed
- **WHEN** migration moves every item out of `~/.exasol/personal` and deletes the legacy cache
- **THEN** `~/.exasol/personal` and the legacy cache's `.exasol` directory no longer exist

#### Scenario: Legacy directory with remaining content is kept
- **WHEN** a legacy directory still holds content after migration
- **THEN** that directory and its content stay in place

#### Scenario: Legacy directory that is the working directory is kept
- **WHEN** the working directory is a legacy directory that migration empties
- **THEN** that directory stays in place

### Requirement: A settings file already at its new location SHALL take precedence
When the cache configuration or the SQL history exists at both its legacy and its new location, the launcher SHALL use the file at the new location.

#### Scenario: Settings file at the new location is used
- **WHEN** the cache configuration and the SQL history each exist at both their legacy and their new location
- **THEN** each file at the new location keeps its content
