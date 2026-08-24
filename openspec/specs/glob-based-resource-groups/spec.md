# glob-based-resource-groups Specification

## Purpose
TBD - created by archiving change embed-presets-via-resource-cache. Update Purpose after archive.
## Requirements
### Requirement: A resource definition can declare itself a glob template
The resource specification format SHALL support a resource-level `glob: true` field. For a
glob template, each declared artifact's `resource_path` SHALL be interpreted as a glob pattern
to match against the artifact's own resolved content, rather than a single literal subpath. A
glob template otherwise resolves through the same fetch, extract, and embed pipeline as any
other resource, as one resource identified by its own resource ID.

#### Scenario: Glob template requires a pattern
- **WHEN** a resource definition sets `glob: true`
- **AND** one of its declared artifacts has an empty `resource_path`
- **THEN** the specification is rejected as invalid

#### Scenario: Non-glob resource is unaffected
- **WHEN** a resource definition omits `glob: true`
- **THEN** its `resource_path`, if any, continues to select a single literal subpath, exactly
  as before this capability existed

### Requirement: A glob template's member is resolved by matching within its resolved root
Resolving a named member of a glob template SHALL resolve the template's own artifact through
the ordinary fetch/extract/embed pipeline, then match the template's `resource_path` pattern
against the result's own entries (files or directories alike), uniformly whether the resolved
root is a local directory, a git checkout, or an extracted archive. A member is never
independently fetched, cached, or embedded — it is only addressed as a subpath of the group.

#### Scenario: Member matches within a local directory
- **WHEN** a glob template's artifact URL resolves to a local directory
- **THEN** resolving a member matches the pattern against that directory's own contents,
  without extraction

#### Scenario: Member matches within a cloned git repository
- **WHEN** a glob template's artifact URL identifies a git repository
- **THEN** resolving a member clones the repository and matches the pattern against the
  clone's contents

#### Scenario: Member matches within an extracted archive
- **WHEN** a glob template's artifact declares `extract: true` and its URL identifies an
  archive
- **THEN** resolving a member extracts the archive and matches the pattern against the
  extracted contents

#### Scenario: A git glob template rejects extraction
- **WHEN** a glob template's artifact URL identifies a git repository
- **AND** the template declares `extract: true`
- **THEN** the specification is rejected as invalid, since a git checkout is already a
  directory with nothing to extract

#### Scenario: File and directory matches are both valid members
- **WHEN** a glob template's pattern matches an entry inside its resolved root
- **THEN** that entry is a valid member regardless of whether it is a file or a directory

#### Scenario: Requesting an unmatched name fails
- **WHEN** a caller requests a member name that does not match the template's pattern within
  its resolved root
- **THEN** resolution fails with an error naming the member and the group

### Requirement: A glob template embedded for real always embeds its matched entries as one archive
For an `embed: true` or `embed: always` glob template, the build-time generator SHALL embed an
archive containing only the entries its own `resource_path` matched within the resolved root —
never the root's full, unfiltered content — and SHALL register the matched member names
alongside the embedded data. A running binary resolving that template from embedded data SHALL
extract the embedded archive regardless of the template's own declared `extract` value, since
the embedded form is always an archive even when the live source needs no extraction to read.

#### Scenario: Embedded archive excludes unmatched entries
- **WHEN** the build-time generator embeds a glob template whose resolved root contains
  entries that do not match its `resource_path`
- **THEN** the embedded archive contains only the matched entries

#### Scenario: Embedded glob template is extracted regardless of its declared extract value
- **WHEN** a glob template declares `extract: false` because its live source is a bare
  directory
- **AND** the template is resolved from embedded data
- **THEN** the running binary extracts the embedded archive before matching a member within it

### Requirement: Build-time-registered member names answer group membership without extraction
For a glob template embedded for real, the build-time generator SHALL register the matched
member names it found, so a running binary can list a group's members without extracting the
embedded archive.

#### Scenario: Listing members does not require extraction
- **WHEN** a running binary lists the members of an embedded glob template's group
- **THEN** it returns the build-time-registered member names without extracting the embedded
  archive

#### Scenario: A group with no registered members reports none
- **WHEN** a running binary lists the members of a group that was never embedded as a glob
  template
- **THEN** it reports no members
