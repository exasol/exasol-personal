# embedded-resource-generation Specification

## Purpose
TBD - created by archiving change add-embedded-resource-management. Update Purpose after archive.
## Requirements
### Requirement: Resources declare whether they are embeddable
The resource specification format SHALL support a resource-level `embed: true` field, applying uniformly to every platform declared under that resource's artifact map.

#### Scenario: Resource marked for embedding
- **WHEN** a resource definition sets `embed: true`
- **THEN** every platform declared under that resource's artifact map is eligible for build-time embedding

#### Scenario: Resource not marked for embedding is unaffected
- **WHEN** a resource definition omits `embed: true`
- **THEN** the resource resolves only through its normal network sources, exactly as before this capability existed

### Requirement: Build-time generation produces embedded resource data for every declared platform
For every resource marked `embed: true`, a build-time generator SHALL fetch and checksum-verify that resource's artifact for its target platform, and SHALL write the result where a build for that platform reads it. Generation SHALL produce data only, so a launcher targeting a platform nothing has been generated for still builds.

#### Scenario: Real artifact data is generated for a declared platform
- **WHEN** the generator runs for an `embed: true` resource and a platform declared under that resource's artifact map
- **THEN** it fetches and checksum-verifies that platform's artifact and writes the fetched bytes where a build for that platform reads them

#### Scenario: A platform with nothing generated still builds
- **WHEN** the launcher is built for a platform the generator has not run for
- **THEN** the build succeeds
- **AND** resources that would have been embedded report that their data is
  absent when they are requested

#### Scenario: Embedded data keeps its source's file extension
- **WHEN** the generator writes an embedded resource's data
- **THEN** the written file carries the extension of the content it holds

#### Scenario: No data is written for an undeclared platform
- **WHEN** the generator runs for an `embed: true` resource targeting a platform not declared under that resource's artifact map
- **THEN** it writes no artifact data for that resource on that platform

#### Scenario: Generation for one platform does not affect another
- **WHEN** the generator runs for one platform of a resource
- **THEN** previously generated output for other platforms of that resource is left unchanged

### Requirement: Build-time generation supports a placeholder-only mode for build speed
Generation SHALL support a mode in which no resource's content is embedded, and the generator performs no fetch or checksum verification for any resource, so that a build needing only a compilable package, not real embedded content, completes without network access.

#### Scenario: Placeholder-only mode skips every real fetch
- **WHEN** generation runs in placeholder-only mode
- **THEN** no resource's content is embedded, regardless of whether
  that resource is marked `embed: true`

#### Scenario: Placeholder-only mode still yields a resolvable specification
- **WHEN** generation runs in placeholder-only mode
- **AND** a resource marked `embed: true` is therefore not embedded
- **THEN** the generated specification declares that resource's upstream source,
  so the launcher fetches it at runtime rather than failing

### Requirement: A resource can require real embedding even under placeholder-only generation
The resource specification format SHALL support an `embed: always` value for the `embed`
field, distinct from `embed: true`. A resource marked `embed: always` SHALL be fetched and
checksum-verified for real and embedded even when generation otherwise runs in placeholder-only
mode.

#### Scenario: An `embed: always` resource is embedded for real under placeholder-only generation
- **WHEN** generation runs in placeholder-only mode
- **AND** a resource is marked `embed: always`
- **THEN** the generator fetches and checksum-verifies that resource's real artifact and embeds
  it, rather than leaving it unembedded

#### Scenario: An `embed: true` resource is not embedded under placeholder-only generation
- **WHEN** generation runs in placeholder-only mode
- **AND** a resource is marked `embed: true` rather than `embed: always`
- **THEN** the generator leaves that resource unembedded instead of fetching real data

### Requirement: Generated embedded resource files are excluded from version control
No resource-specific generated file SHALL be committed to the repository.

#### Scenario: Generated output is gitignored
- **WHEN** the generator writes embedded resource files
- **THEN** those files are written to a location excluded from version control

### Requirement: The generator always fetches independently of previously embedded data
The build-time generator SHALL resolve each resource's artifact through the same fetch-and-verify path used for any network resource, without consulting any registry of previously embedded data.

#### Scenario: Generator fetch is never satisfied by embedded data
- **WHEN** the generator fetches an artifact for an `embed: true` resource
- **THEN** it performs a real fetch and checksum verification, regardless of whether that resource's data has previously been embedded in any binary

### Requirement: An embedded resource's cache identity comes from its build-time content
An embedded resource's cache identity SHALL be the content hash the build-time generator computes over the bytes it actually embedded, recorded in the generated specification.

#### Scenario: Different embedded content produces different cache identity
- **WHEN** two builds embed different content under the same resource ID
- **THEN** each build's binary resolves that resource ID to its own, distinct cache entry

#### Scenario: An upgraded binary does not reuse a stale cache entry
- **WHEN** a binary embedding new content for a resource ID runs after a previous binary
  already cached that resource ID's old content
- **THEN** the new binary resolves and caches its own content rather than reusing the old
  cache entry

#### Scenario: Identical generated content produces an identical identity
- **WHEN** the generator runs twice on identical source content, on different
  machines
- **THEN** it records the same identity for that resource both times

### Requirement: Built-in preset directories are embedded through generation
Built-in infrastructure and installation preset directories SHALL be embedded through the build-time resource generator as one independently addressable resource per preset, not through native Go source embedding.

#### Scenario: A new preset directory needs no source-code edit
- **WHEN** a new built-in preset directory is added under the existing preset catalog entry's
  matching location
- **THEN** the build-time generator embeds it without any change to Go source code

#### Scenario: Preset content resolves through the resource cache at runtime
- **WHEN** the launcher reads a built-in preset directory at runtime
- **THEN** it resolves that directory through the resource cache, exactly as any other
  embedded resource

#### Scenario: Reading one preset does not materialize the others
- **WHEN** the launcher reads one built-in preset directory
- **THEN** no other preset directory in the same catalog is materialized

### Requirement: A build SHALL embed only what its resolved specification references
Generation SHALL leave a platform's embedded data holding exactly what that platform's resolved specification references. Data written by an earlier generation that the current one does not reference SHALL NOT reach the build. Pruning SHALL be confined to the platform being generated.

#### Scenario: Data for a resource no longer declared is not embedded
- **WHEN** a resource that was embedded previously is removed from the
  specification, and generation runs again for that platform
- **THEN** that resource's data is no longer present for the build to embed

#### Scenario: Data skipped by placeholder-only mode is not embedded
- **WHEN** generation has previously embedded a resource for a platform
- **AND** generation runs again for that platform in placeholder-only mode
- **THEN** that resource's data is no longer present for the build to embed

#### Scenario: Pruning is confined to the target platform
- **WHEN** generation runs for one platform
- **THEN** data generated for other platforms is left in place

### Requirement: Build-time generation SHALL produce a fully concrete resource specification
The build-time generator SHALL emit, for its target platform, a resource specification in which every resource states one source, one identity, and one presentation, with no embedding directive and no expansion pattern remaining. The launcher SHALL resolve resources from that generated specification.

#### Scenario: Generated specification contains no build directives
- **WHEN** the generator produces a specification for a platform
- **THEN** no resource in it declares an embedding directive or an expansion
  pattern

#### Scenario: Embedded resource points at embedded data
- **WHEN** the generator embeds a resource's content
- **THEN** the generated specification declares that resource's source as
  `embedded://`

#### Scenario: Resource that was not embedded points at its upstream source
- **WHEN** the generator does not embed a resource's content
- **THEN** the generated specification declares that resource's original upstream
  source

#### Scenario: A local source that is not embedded is rejected
- **WHEN** a resource declares a local source and the generator does not embed it
- **THEN** generation fails with an error naming that resource

  A local path names a location in the checkout that generated it, which a
  launcher running anywhere else cannot reach, whether the path is relative or
  absolute.
