## ADDED Requirements

### Requirement: Install an official script language container by alias

`exasol slc install <alias>` SHALL resolve the alias against the official SLC catalog and
make the corresponding language available in the local deployment, without the user running
any SQL.

#### Scenario: Install a known alias

- **WHEN** `exasol slc install python3` is run against a local deployment
- **THEN** the resolved official SLC is mounted into the database
- **AND** the command reports success only after the database is ready with the SLC active

#### Scenario: Alias matching is case-insensitive

- **WHEN** `exasol slc install PYTHON3` and `exasol slc install python3` are run
- **THEN** both resolve to the same catalog entry

#### Scenario: Unknown alias is rejected

- **WHEN** `exasol slc install nodejs` is run and `nodejs` is not in the catalog
- **THEN** the command fails with an error listing the valid aliases
- **AND** no deployment state is changed and no restart occurs

#### Scenario: Unsupported on non-local backends

- **WHEN** `exasol slc install python3` is run against a non-local deployment
- **THEN** the command fails with a clear "unsupported" message
- **AND** no deployment state is changed

#### Scenario: Not-yet-deployed deployment is refused

- **WHEN** an SLC change (`install`, `update`, or `remove`) is run against a deployment that is initialized but not deployed yet
- **THEN** the command fails asking the user to run `exasol deploy` first
- **AND** no deployment state is changed and no SLC is recorded

#### Scenario: Installing an already-installed image is a no-op

- **WHEN** `exasol slc install python3` is run and the resolved image is already installed
- **THEN** the command reports it is already installed and up to date
- **AND** no deployment state is changed and no restart occurs

### Requirement: Installed languages activate without manual SQL

After a successful install, the language SHALL be usable through the database's built-in
alias mechanism, with no `ALTER SYSTEM` or other manual SQL.

#### Scenario: Installed Python is usable

- **WHEN** `exasol slc install python3` has completed successfully
- **THEN** a `CREATE PYTHON3 SCALAR SCRIPT ...` statement succeeds and the script runs

### Requirement: Alias uniqueness is enforced across installed SLCs

The launcher SHALL keep the installed SLC set disjoint across all declared aliases — both
unversioned (e.g. `PYTHON3`) and versioned (e.g. `PYTHON312`) — because the database fails
to start if two installed SLCs declare the same alias. An install that would introduce a
duplicate alias SHALL be rejected, except a newer version of an already-installed flavor,
which replaces it.

#### Scenario: Conflicting install is rejected

- **WHEN** an SLC exposing `PYTHON3` is already installed
- **AND** the user installs a different flavor that also exposes `PYTHON3`
- **THEN** the command fails naming the conflicting alias
- **AND** the existing installation and deployment state are unchanged

#### Scenario: Any shared alias conflicts, not only the unversioned one

- **WHEN** an installed SLC and a candidate SLC of a different flavor share any declared alias
- **THEN** the install is rejected naming the shared alias

#### Scenario: Same-flavor version change replaces the incumbent

- **WHEN** an SLC for a flavor is already installed
- **AND** the user installs a newer version of the same flavor
- **THEN** the newer version replaces the existing one rather than being added alongside it

### Requirement: Install, update, and remove apply through a verified database restart

`exasol slc install`, `exasol slc update`, and `exasol slc remove` SHALL restart the local
database to apply the change, and SHALL report success only after the database is ready with
the change in effect.

#### Scenario: Install restarts and verifies

- **WHEN** `exasol slc install python3` is run on a running deployment
- **THEN** the database is restarted with the SLC mounted
- **AND** success is reported only after readiness and activation are verified

#### Scenario: A failed apply does not report success

- **WHEN** the database fails to come up with the SLC mounted (e.g. the image cannot be pulled)
- **THEN** the command reports the failure
- **AND** it indicates the SLC is configured but not active, rather than reporting success

### Requirement: A restart of a running database is confirmed or deferred

`exasol slc install`, `exasol slc update`, and `exasol slc remove` SHALL require confirmation
before restarting a running database, and SHALL offer `--yes` to skip the prompt and
`--no-restart` to record the change for the next start instead of restarting now.

#### Scenario: Confirmation is required before restarting a running database

- **WHEN** `exasol slc install python3` is run interactively on a running deployment
- **THEN** the command warns that the database will be restarted and open connections dropped
- **AND** it proceeds only after the user confirms; declining makes no changes

#### Scenario: `--yes` skips the prompt

- **WHEN** `exasol slc install python3 --yes` is run on a running deployment
- **THEN** the database is restarted to apply the SLC without prompting

#### Scenario: `--no-restart` defers activation without restarting

- **WHEN** `exasol slc install python3 --no-restart` is run on a running deployment
- **THEN** the SLC is recorded and the database is not restarted
- **AND** the SLC becomes active on the next start

#### Scenario: Non-interactive use without confirmation is refused

- **WHEN** `exasol slc install python3` is run without a TTY and without `--yes` or `--no-restart` on a running deployment
- **THEN** the command fails asking for `--yes` or `--no-restart`
- **AND** the database is not restarted

#### Scenario: No confirmation when the database is stopped

- **WHEN** `exasol slc install python3` is run on a stopped deployment
- **THEN** no restart confirmation is required, because no running database is disrupted

### Requirement: Unreferenced SLC images are reclaimed

Replacing or removing an SLC SHALL NOT leave the replaced or removed image occupying
storage indefinitely; the launcher SHALL remove SLC images that are no longer referenced by
the installed set. Reclamation MUST be limited to official SLC images and MUST NOT remove
the database image or any unrelated image, and a failure to remove an image MUST NOT fail
the operation.

#### Scenario: Replacing an SLC removes the old image

- **WHEN** an installed SLC is replaced by a newer version of the same flavor
- **THEN** the newer image is mounted
- **AND** the previous, now-unreferenced SLC image is removed from storage

#### Scenario: Removing an SLC removes its image

- **WHEN** an installed SLC is removed
- **THEN** its image is removed from storage on the next database (re)start

#### Scenario: Images still in use are left in place

- **WHEN** an SLC image cannot be removed because it is still referenced
- **THEN** the removal is skipped without failing the install or remove

### Requirement: Update an installed SLC to the catalog's current version

`exasol slc update <alias>` SHALL re-resolve the alias against the catalog and compare the
resolved image with the installed one. When the resolved image is unchanged the command
SHALL be a no-op with no restart; when it has changed the command SHALL replace the installed
SLC and apply it through a database restart. Update SHALL NOT order versions or guard against
"older" images — rollback is out of scope, so it installs whatever the catalog resolves to.

#### Scenario: Update with no catalog change is a no-op

- **WHEN** `exasol slc update python3` is run and the resolved image matches the installed one
- **THEN** the command reports it is already up to date
- **AND** no deployment state is changed and no restart occurs

#### Scenario: Update applies a changed image

- **WHEN** `exasol slc update python3` is run and the catalog resolves to a different image
- **THEN** the installed SLC is replaced with the newly resolved one
- **AND** success is reported only after the database is ready with the new image active

#### Scenario: Update of a not-installed SLC

- **WHEN** `exasol slc update python3` is run and `python3` is not installed
- **THEN** the command reports that nothing is installed for that alias
- **AND** no restart occurs

### Requirement: List available and installed SLCs

`exasol slc list` SHALL show the official SLCs available in the catalog and which of them are
currently installed.

#### Scenario: List reflects install state

- **WHEN** `exasol slc list` is run before and after installing `python3`
- **THEN** the entry for `python3` is shown as not installed before, and installed after

#### Scenario: Unsupported architecture yields an empty list, not an error

- **WHEN** `exasol slc list` is run where the catalog has no SLCs for the current architecture
- **THEN** the command reports that no containers are available (empty text message, `[]` in JSON)
- **AND** it exits successfully rather than failing, unlike install/update/remove

### Requirement: Remove an installed SLC

`exasol slc remove <alias>` SHALL remove the SLC from the installed set and deactivate the
language after the database restarts.

#### Scenario: Remove an installed SLC

- **WHEN** `python3` is installed and `exasol slc remove python3` is run
- **THEN** the SLC is removed and, after restart, `PYTHON3` is no longer available

#### Scenario: Remove a not-installed SLC

- **WHEN** `exasol slc remove python3` is run and `python3` is not installed
- **THEN** the command reports that nothing was installed for that alias
- **AND** no restart occurs
