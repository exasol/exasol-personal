# local-slc-management Specification

## Purpose
Define how script language containers (SLCs) are installed, updated, listed, and removed in local deployments, covering both the official SLC catalog and user-supplied custom containers, so that UDFs in a given language become usable without the user running manual SQL.
## Requirements
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

The launcher SHALL keep aliases disjoint across all installed SLCs — official and custom —
across all declared aliases, both unversioned (e.g. `PYTHON3`) and versioned (e.g.
`PYTHON312`). Because the database fails to start if two installed official SLCs declare the
same alias, an official install that would introduce a duplicate official alias SHALL be
rejected, except a newer version of an already-installed flavor, which replaces it. An
official install SHALL additionally be blocked when a custom SLC already owns one of its
aliases, guiding the user to remove the custom SLC first.

#### Scenario: Conflicting official install is rejected

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

#### Scenario: Official install blocked when a custom SLC owns the alias

- **WHEN** a custom SLC owns `PYTHON3`
- **AND** the user installs an official SLC that also declares `PYTHON3`
- **THEN** the install is blocked, guiding the user to remove the custom SLC first

### Requirement: Install, update, and remove apply through a verified database restart

An SLC operation that applies through a restart SHALL restart the local database to apply the
change, and SHALL report success only after the database is ready with the change in effect.
This covers official install, update, and remove, and custom install and update. For a custom container, the change is in effect only once its alias resolves to the
mounted container in `SCRIPT_LANGUAGES`.

#### Scenario: Install restarts and verifies

- **WHEN** `exasol slc install python3` is run on a running deployment
- **THEN** the database is restarted with the SLC mounted
- **AND** success is reported only after readiness and activation are verified

#### Scenario: A failed apply does not report success

- **WHEN** the database fails to come up with the SLC mounted (e.g. the image cannot be pulled)
- **THEN** the command reports the failure
- **AND** it indicates the SLC is configured but not active, rather than reporting success

#### Scenario: A custom install restarts and verifies activation

- **WHEN** `exasol slc custom install` is run on a running deployment
- **THEN** the database is restarted with the container mounted, and the alias is activated
- **AND** success is reported only after the alias is confirmed to resolve to the container

#### Scenario: A custom container that could not be activated is reported, not claimed

- **WHEN** a custom install restarts the database but the container is unavailable or the alias cannot be activated
- **THEN** the command reports that the container is recorded but not active
- **AND** the deployment remains usable

### Requirement: A restart of a running database is confirmed or deferred

Every SLC operation that restarts the database SHALL require confirmation before restarting a
running database, and SHALL offer `--auto-approve` to skip the prompt and `--no-restart` to
record the change for the next start instead of restarting now. This covers official install,
update, and remove, and custom install and update. Custom removal does not restart the
database, so it offers neither flag.

#### Scenario: Confirmation is required before restarting a running database

- **WHEN** `exasol slc install python3` is run interactively on a running deployment
- **THEN** the command warns that the database will be restarted and open connections dropped
- **AND** it proceeds only after the user confirms; declining makes no changes

#### Scenario: `--auto-approve` skips the prompt

- **WHEN** `exasol slc install python3 --auto-approve` is run on a running deployment
- **THEN** the database is restarted to apply the SLC without prompting

#### Scenario: `--no-restart` defers activation without restarting

- **WHEN** `exasol slc install python3 --no-restart` is run on a running deployment
- **THEN** the SLC is recorded and the database is not restarted
- **AND** the SLC becomes active on the next start

#### Scenario: `--no-restart` defers a custom install without restarting

- **WHEN** `exasol slc custom install --no-restart` is run on a running deployment
- **THEN** the container is staged and recorded, and the database is not restarted
- **AND** the container is mounted and its alias activated on the next start

#### Scenario: Non-interactive use without confirmation is refused

- **WHEN** `exasol slc install python3` is run without a TTY and without `--auto-approve` or `--no-restart` on a running deployment
- **THEN** the command fails asking for `--auto-approve` or `--no-restart`
- **AND** the database is not restarted

#### Scenario: No confirmation when the database is stopped

- **WHEN** `exasol slc install python3` is run on a stopped deployment
- **THEN** no restart confirmation is required, because no running database is disrupted

### Requirement: Unreferenced SLC images are reclaimed

Replacing or removing an SLC SHALL NOT leave the replaced or removed image occupying
storage indefinitely; SLC images that are no longer referenced by the installed set SHALL
be reclaimed, whether they were pulled for an official SLC or imported for a custom one.
Reclamation MUST NOT remove the database image or any unrelated image, and a failure to remove
an image MUST NOT fail the operation. A replaced or removed custom container's staged package
SHALL also be deleted, so a superseded container does not occupy host storage.

#### Scenario: Replacing an SLC removes the old image

- **WHEN** an installed SLC is replaced by a newer version of the same flavor
- **THEN** the newer image is mounted
- **AND** the previous, now-unreferenced SLC image is removed from storage

#### Scenario: Removing an SLC removes its image

- **WHEN** an installed SLC is removed
- **THEN** its image is removed from storage on the next database (re)start

#### Scenario: Removing a custom SLC reclaims its image and package

- **WHEN** a custom SLC is removed or replaced with different content
- **THEN** its staged package is deleted from the host
- **AND** its imported image is removed from storage on the next database (re)start

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

### Requirement: Install a custom script language container

`exasol slc custom install` SHALL install a user-supplied container given a source and an
alias: `--source <tarball-or-https-url>`, with `--alias <NAME>` and
`--language <python|java|r>`. The launcher SHALL deliver the container by staging it for the
local runtime to import and mount into the database container, and SHALL activate it by setting
the alias in the `SCRIPT_LANGUAGES` database parameter, preserving every other alias. The
operation SHALL be supported only on local deployments.

The alias MUST be a valid unquoted Exasol regular identifier restricted to ASCII: it starts
with a letter, then letters, digits, or underscores, up to 128 characters — so it works in
`CREATE <alias> SCALAR SCRIPT`. Reserved words are still rejected by the database at use time.
See https://docs.exasol.com/db/latest/sql_references/basiclanguageelements.htm

#### Scenario: Install from a local tarball

- **WHEN** the user runs `slc custom install --source c.tar.gz --alias MYPY3 --language python`
- **THEN** the container is staged for the local runtime and mounted into the database container
- **AND** `SCRIPT_LANGUAGES` gains a `MYPY3` entry while all existing aliases are preserved
- **AND** the language is usable once the database is ready

#### Scenario: Install from a URL does not leave the download on disk

- **WHEN** the user installs with an `https` URL as the source
- **THEN** the container is downloaded on the host and staged for the local runtime
- **AND** the downloaded copy is removed once it has been staged
- **AND** a URL source is rejected unless it uses `https`

#### Scenario: Alias and language are validated

- **WHEN** the alias is empty, does not start with a letter, exceeds 128 characters, or contains characters other than letters, digits, and underscores
- **THEN** the command is rejected before any download
- **WHEN** the language is not one of python, java, or r
- **THEN** the command is rejected

#### Scenario: A source is required

- **WHEN** `--source` is not given
- **THEN** the command is rejected

#### Scenario: An invalid container is rejected before it is staged

- **WHEN** the supplied archive is corrupt, or is not a standard script language container
- **THEN** the command fails before changing the deployment or the database

#### Scenario: Activation is confirmed before success is reported

- **WHEN** the activation of a custom SLC does not take effect
- **THEN** the command reports an error instead of reporting success

#### Scenario: Reinstalling identical content and language is a no-op

- **WHEN** the requested container has the same content digest and language as the one already installed and active under that alias
- **THEN** the command makes no change and reports that nothing was done
- **WHEN** the content digest matches but the language differs
- **THEN** the alias is re-activated with the new language and the change is recorded
- **WHEN** the content digest and language match but the recorded container was never activated
- **THEN** the install is retried rather than reported as a no-op

### Requirement: A custom container is addressed by its alias and content

The launcher SHALL derive a custom container's image reference, mount directory, and staged
package name from its alias and content digest, and SHALL keep custom mount directories in a
namespace disjoint from official mount directories and from the directories the database owns.
Changing a custom container's content SHALL therefore change its image reference, so the local
runtime imports the new content rather than reusing the old image.

#### Scenario: Custom and official containers cannot collide on a mount directory

- **WHEN** a custom SLC is installed under any alias
- **THEN** its mount directory is distinct from every official SLC mount directory and from the database's own directories

#### Scenario: New content produces a new image

- **WHEN** a custom alias is updated with different content
- **THEN** the image reference changes with the content digest
- **AND** the local runtime imports the new content instead of reusing the previously imported image

### Requirement: Pending custom activation is reconciled on start

The launcher SHALL record a custom container's activation as pending until it has been applied,
and SHALL apply any pending activation once the database is ready after a start — because the
container is mounted by the start path but activated through the database. A failure to
reconcile SHALL be reported without failing the start.

#### Scenario: A deferred install activates on the next start

- **WHEN** a custom SLC was recorded with `--no-restart`
- **AND** the deployment is started
- **THEN** the container is mounted and its alias is activated once the database is ready
- **AND** no further user action is required

#### Scenario: An interrupted install converges on the next start

- **WHEN** a custom SLC was recorded and mounted but its activation did not complete
- **AND** the deployment is started again
- **THEN** the activation is applied

#### Scenario: A reconcile failure does not fail the start

- **WHEN** a pending activation cannot be applied during a start
- **THEN** the failure is reported as a warning
- **AND** the database remains started

### Requirement: An unavailable custom container does not stop the database

The local runtime SHALL report a custom container whose staged package is missing or cannot be
imported as unavailable, skip mounting it, and still start the database. The launcher SHALL NOT
activate an alias whose container is unavailable, and SHALL tell the user which container is
unavailable and why.

#### Scenario: A missing package leaves the database usable

- **WHEN** a recorded custom container's staged package is missing at start
- **THEN** the database starts without that container
- **AND** the alias is not activated
- **AND** the user is told the container is unavailable

#### Scenario: A container that cannot be imported leaves the database usable

- **WHEN** a staged package is not a valid container image archive
- **THEN** the database starts without that container
- **AND** the command that installed it reports the failure rather than success

### Requirement: Custom SLCs are tracked separately from official ones

The launcher SHALL persist installed custom SLCs in a state list separate from official SLCs,
recording each container's image reference, mount target, staged package name, content digest,
source, and whether its activation has been applied. This state SHALL be the source of truth
that the start path uses to re-apply custom mounts, because an image mount does not survive
container recreation.

#### Scenario: Start re-applies custom SLCs as mounts

- **WHEN** a deployment has both an official and a custom SLC installed
- **AND** the deployment is started
- **THEN** the official SLC contributes an image mount
- **AND** the custom SLC contributes an image mount together with the staged package the runtime must import

### Requirement: Custom and official aliases are mutually exclusive

An alias SHALL have a single owner across custom and official SLCs. When installing a custom
SLC whose alias is owned by an installed official SLC, the command SHALL be blocked and
guide the user to remove the official SLC or choose another alias. When the alias is a
built-in/official name that is not currently installed, the command SHALL require
confirmation before overriding it. When installing a custom SLC whose alias already belongs
to another installed custom SLC, the command SHALL require confirmation before replacing it.

#### Scenario: Blocked when an official SLC owns the alias

- **WHEN** an official SLC providing `PYTHON3` is installed
- **AND** the user installs a custom SLC with `--alias PYTHON3`
- **THEN** the command is blocked, naming the official SLC to remove or asking for a different alias

#### Scenario: Overriding a built-in alias is confirmed

- **WHEN** no official SLC is installed for `PYTHON3`
- **AND** the user installs a custom SLC with `--alias PYTHON3`
- **THEN** the command asks for confirmation before overriding the built-in, and `--auto-approve` skips the prompt

#### Scenario: Replacing an installed custom SLC is confirmed

- **WHEN** a custom SLC is installed under an alias
- **AND** the user installs different content under the same alias
- **THEN** the command asks for confirmation before replacing it

### Requirement: Manage custom SLCs through list, update, and remove

`exasol slc list` SHALL show installed custom SLCs alongside official ones, distinguished by
type, and SHALL report whether each custom container is currently available. `exasol slc custom
update` SHALL replace the container behind a custom alias with a freshly supplied one, treating
identical content and language as a no-op. `exasol slc remove <alias>` SHALL remove either an
official or a custom container, deactivating a custom one — removing its `SCRIPT_LANGUAGES`
entry, or restoring the built-in mapping it displaced — and deleting its staged package. Removing an *active* container SHALL require a running database,
because clearing its alias goes through the database; a container that was recorded but never
activated holds no alias and SHALL be removable while the database is stopped.

#### Scenario: List includes custom SLCs

- **WHEN** a custom SLC is installed
- **THEN** `slc list` shows it with its alias, language, and availability, and `--json` marks it with a custom type

#### Scenario: Remove a custom SLC

- **WHEN** the user runs `slc remove` for a custom alias
- **THEN** the custom SLC is deactivated and its staged package is deleted
- **AND** its mount is gone from the next start
- **AND** the same alias removed through the official path is not mistaken for an official SLC

#### Scenario: Removing a custom SLC restores a displaced built-in

- **WHEN** a custom SLC that overrode a built-in alias is removed
- **THEN** the built-in mapping is restored rather than the alias being left undefined

#### Scenario: Removing an active container with a stopped database is refused

- **WHEN** `slc custom remove` is run for an active container while the database is stopped
- **THEN** the command fails asking the user to start the deployment first

#### Scenario: A container that was never activated is removable while stopped

- **WHEN** `slc custom remove` is run for a container recorded but never activated, while the database is stopped
- **THEN** the container is removed and its staged package deleted, without requiring a start

#### Scenario: Update replaces custom content

- **WHEN** the user updates a custom alias with new content
- **THEN** the container is replaced and re-activated, and identical content and language is reported as a no-op
