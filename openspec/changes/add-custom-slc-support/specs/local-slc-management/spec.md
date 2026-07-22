## ADDED Requirements

### Requirement: Install a custom script language container

`exasol slc custom install` SHALL install a user-supplied container given a source and an
alias: `--file <tarball>` or `--url <https-url>` (exactly one), with `--alias <NAME>`
and `--language <python|java|r>`. The launcher SHALL materialize the container into the
default BucketFS bucket and activate it by setting the alias in the `SCRIPT_LANGUAGES`
database parameter, preserving every other alias. Activation SHALL take effect for new
sessions without restarting the database. The operation SHALL be supported only on local
deployments and SHALL require a running database.

The alias MUST be a valid unquoted Exasol regular identifier restricted to ASCII: it starts
with a letter, then letters, digits, or underscores, up to 128 characters — so it works in
`CREATE <alias> SCALAR SCRIPT`. Reserved words are still rejected by the database at use time.
See https://docs.exasol.com/db/latest/sql_references/basiclanguageelements.htm

#### Scenario: Install from a local tarball

- **WHEN** the user runs `slc custom install --file c.tar.gz --alias MYPY3 --language python`
- **THEN** the container is unpacked into the default BucketFS bucket
- **AND** `SCRIPT_LANGUAGES` gains a `MYPY3` entry while all existing aliases are preserved
- **AND** the language is usable in new sessions without a restart

#### Scenario: Install from a URL does not leave the download on disk

- **WHEN** the user installs with `--url`
- **THEN** the container is downloaded on the host and streamed into the deployment
- **AND** the downloaded copy is removed after it has been unpacked
- **AND** the URL is rejected unless it uses `https`

#### Scenario: Alias and language are validated

- **WHEN** the alias is empty, does not start with a letter, exceeds 128 characters, or contains characters other than letters, digits, and underscores
- **THEN** the command is rejected before any download
- **WHEN** the language is not one of python, java, or r
- **THEN** the command is rejected

#### Scenario: Exactly one source is required

- **WHEN** neither `--file` nor `--url` is given, or both are given
- **THEN** the command is rejected

#### Scenario: An invalid container is rejected before it is installed

- **WHEN** the supplied archive is corrupt, or is not a standard script language container
- **THEN** the command fails before changing the deployment or the database

#### Scenario: Activation is confirmed before success is reported

- **WHEN** the activation of a custom SLC does not take effect
- **THEN** the command reports an error instead of reporting success

#### Scenario: A stopped database is refused

- **WHEN** a custom SLC operation is attempted while the database is stopped
- **THEN** the command fails asking the user to start the deployment first

#### Scenario: Reinstalling identical content and language is a no-op

- **WHEN** the requested container has the same content digest and language as the one already installed under that alias
- **THEN** the command makes no change and reports that nothing was done
- **WHEN** the content digest matches but the language differs
- **THEN** the alias is re-activated with the new language and the change is recorded

### Requirement: Custom SLCs are tracked separately from official ones

The launcher SHALL persist installed custom SLCs in a state list separate from official
SLCs. The mechanism that re-applies official image mounts on every start SHALL NOT include
custom SLCs, because custom SLCs persist through BucketFS and `SCRIPT_LANGUAGES` and are not
re-applied.

#### Scenario: Start does not re-apply custom SLCs as image mounts

- **WHEN** a deployment has both an official and a custom SLC installed
- **AND** the deployment is started
- **THEN** only the official SLC contributes an image mount
- **AND** the custom SLC is not passed as an image mount

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
type. `exasol slc custom update` SHALL replace the container behind a custom alias with a
freshly supplied one, treating identical content and language as a no-op. `exasol slc custom
remove` SHALL deactivate a custom SLC (removing its `SCRIPT_LANGUAGES` entry) and delete its
BucketFS files. The top-level `slc remove` SHALL point a custom alias at `slc custom remove`.

#### Scenario: List includes custom SLCs

- **WHEN** a custom SLC is installed
- **THEN** `slc list` shows it with its alias and language, and `--json` marks it with a custom type

#### Scenario: Remove a custom SLC

- **WHEN** the user runs `slc custom remove` for a custom alias
- **THEN** the custom SLC is deactivated and its files are deleted
- **WHEN** the user runs the top-level `slc remove` for a custom alias
- **THEN** the command points them at `slc custom remove` instead of removing an official SLC

#### Scenario: Update replaces custom content

- **WHEN** the user updates a custom alias with new content
- **THEN** the container is replaced and re-activated, and identical content and language is reported as a no-op

## MODIFIED Requirements

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
