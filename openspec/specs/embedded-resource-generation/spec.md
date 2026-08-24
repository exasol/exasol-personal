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
For every resource marked `embed: true`, a build-time generator SHALL fetch and checksum-verify that resource's artifact for each of its declared platforms, and SHALL write the result into a build-tag-gated, generated source file scoped to that platform. For any platform not declared for that resource, the generator SHALL still produce a build-tag-gated file containing no data, so the containing package compiles for every target the project builds.

#### Scenario: Real artifact data is generated for a declared platform
- **WHEN** the generator runs for an `embed: true` resource and a platform declared under that resource's artifact map
- **THEN** it fetches and checksum-verifies that platform's artifact and writes the fetched bytes into a generated file scoped to that platform

#### Scenario: Placeholder data is generated for an undeclared platform
- **WHEN** the generator runs for an `embed: true` resource targeting a platform not declared under that resource's artifact map
- **THEN** it writes a generated file scoped to that platform containing no artifact data

#### Scenario: Generation for one platform does not affect another
- **WHEN** the generator runs for one platform of a resource
- **THEN** previously generated files for other platforms of that resource are left unchanged

### Requirement: Build-time generation supports a placeholder-only mode for build speed
Generation SHALL support a mode in which every resource's generated file contains no artifact
data, and the generator performs no fetch or checksum verification for any resource, so that a
build needing only a compilable package — not real embedded content — completes without
network access.

#### Scenario: Placeholder-only mode skips every real fetch
- **WHEN** generation runs in placeholder-only mode
- **THEN** every resource's generated file contains no artifact data, regardless of whether
  that resource is marked `embed: true`

### Requirement: A resource can require real embedding even under placeholder-only generation
The resource specification format SHALL support an `embed: always` value for the `embed`
field, distinct from `embed: true`. A resource marked `embed: always` SHALL be fetched and
checksum-verified for real and embedded even when generation otherwise runs in placeholder-only
mode.

#### Scenario: An `embed: always` resource is embedded for real under placeholder-only generation
- **WHEN** generation runs in placeholder-only mode
- **AND** a resource is marked `embed: always`
- **THEN** the generator fetches and checksum-verifies that resource's real artifact and embeds
  it, rather than writing a placeholder

#### Scenario: An `embed: true` resource still receives a placeholder under placeholder-only generation
- **WHEN** generation runs in placeholder-only mode
- **AND** a resource is marked `embed: true` rather than `embed: always`
- **THEN** the generator writes a placeholder for that resource instead of fetching real data

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
An `embed: true` resource's cache identity SHALL be derived from a content hash computed by
the build-time generator over the resource's actual embedded bytes, not from a runtime
path-based hash.

#### Scenario: Different embedded content produces different cache identity
- **WHEN** two builds embed different content under the same resource ID
- **THEN** each build's binary resolves that resource ID to its own, distinct cache entry

#### Scenario: An upgraded binary does not reuse a stale cache entry
- **WHEN** a binary embedding new content for a resource ID runs after a previous binary
  already cached that resource ID's old content
- **THEN** the new binary resolves and caches its own content rather than reusing the old
  cache entry

### Requirement: Built-in preset directories are embedded through generation
Built-in infrastructure and installation preset directories SHALL be embedded through the
build-time resource generator, not through native Go source embedding.

#### Scenario: A new preset directory needs no source-code edit
- **WHEN** a new built-in preset directory is added under the existing preset catalog entry's
  matching location
- **THEN** the build-time generator embeds it without any change to Go source code

#### Scenario: Preset content resolves through the resource cache at runtime
- **WHEN** the launcher reads a built-in preset directory at runtime
- **THEN** it resolves that directory through the resource cache, exactly as any other
  `embed: true` resource
