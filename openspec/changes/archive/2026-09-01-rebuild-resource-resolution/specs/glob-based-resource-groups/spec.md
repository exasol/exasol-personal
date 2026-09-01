## MODIFIED Requirements

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

#### Scenario: Repository metadata is not a member
- **WHEN** a glob pattern is matched against a cloned git repository's own root
- **THEN** the repository's metadata directory does not become a member

## ADDED Requirements

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

## REMOVED Requirements

### Requirement: A glob template's member is resolved by matching within its resolved root
**Reason**: A glob pattern is now matched once at build time, so the group has no
runtime representation and there is nothing to match a member against. Members
are resources in their own right, resolved by name.
**Migration**: None required. A member is requested by the same name as before,
and resolves to content with the same layout.

### Requirement: A glob template embedded for real always embeds its matched entries as one archive
**Reason**: Each matched entry is now embedded as its own resource, so there is no
group archive to filter, and no need to override a group's declared extraction
behavior when reading it back.
**Migration**: None required. Built-in preset content resolves to the same
directory layout as before.

### Requirement: Build-time-registered member names answer group membership without extraction
**Reason**: Member names are now resources in the generated specification, so
listing reads the specification directly and no separate registry exists.
**Migration**: None required. Listing reports the same member names.
