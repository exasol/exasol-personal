# glob-based-resource-groups Specification

## Purpose
TBD - created by archiving change embed-presets-via-resource-cache. Update Purpose after archive.
## Requirements
### Requirement: A resource definition can declare itself a glob template
The resource specification format SHALL support a resource-level `glob` field holding a glob pattern. At build time, a resource declaring a pattern SHALL expand into one independently addressable resource per matched entry, named `<group>/<member>`, and the group itself SHALL NOT appear in the generated specification.

#### Scenario: Glob pattern expands into one resource per match
- **WHEN** a resource definition declares a glob pattern
- **AND** the pattern matches several entries within the resource's resolved
  content
- **THEN** the generated specification contains one resource per matched entry,
  named for the group and the entry

#### Scenario: Glob template requires a pattern
- **WHEN** a resource definition declares an empty `glob` pattern
- **THEN** the specification is rejected as invalid

#### Scenario: Non-glob resource is unaffected
- **WHEN** a resource definition omits `glob`
- **THEN** its `subpath`, if any, continues to select a single literal subpath

#### Scenario: File and directory matches are both valid members
- **WHEN** a glob pattern matches an entry inside its resolved content
- **THEN** that entry becomes a member resource regardless of whether it is a
  file or a directory

#### Scenario: Nested matches use their entry names
- **WHEN** a glob pattern matches an entry below a nested directory
- **THEN** the generated member uses the matched entry's base name as its
  member name
- **AND** its selected subpath preserves the full match path within the source

#### Scenario: Repeated entry names report an ambiguity
- **WHEN** a glob pattern matches entries in different directories with the
  same base name
- **THEN** generation reports that the matches share a member name

#### Scenario: Download path selects the generator's extractor
- **WHEN** a glob template declares a `download_path`
- **THEN** the generator uses that path to select the extractor before matching
  the glob pattern

#### Scenario: Repository metadata is not a member
- **WHEN** a glob pattern is matched against a cloned git repository's own root
- **THEN** the repository's metadata directory does not become a member

### Requirement: A group's members SHALL be listed from the resource specification
Listing the members of a group SHALL report every resource in the specification named under that group, without materializing, extracting, or matching anything at runtime.

#### Scenario: Listing members materializes nothing
- **WHEN** a caller lists the members of a group
- **THEN** the launcher reports the member names without materializing any of
  them

#### Scenario: Listing an unknown group reports no members
- **WHEN** a caller lists the members of a group that the specification does not
  declare
- **THEN** the launcher reports no members

### Requirement: A group member SHALL resolve independently of its siblings
Each member of an expanded group SHALL be a resource in its own right, with its own identity and its own cached artifact. Resolving one member SHALL NOT materialize any other member.

#### Scenario: Resolving one member leaves siblings unmaterialized
- **WHEN** a caller resolves one member of a group
- **THEN** no other member of that group is materialized

#### Scenario: Requesting an unknown member fails
- **WHEN** a caller requests a member name that the specification does not
  declare under that group
- **THEN** resolution fails with an error naming the member and the group

#### Scenario: Members are cleaned independently
- **WHEN** cache cleanup selects one member of a group
- **THEN** the other members of that group remain cached
