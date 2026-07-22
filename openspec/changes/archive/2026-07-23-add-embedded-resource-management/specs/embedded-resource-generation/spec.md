## ADDED Requirements

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
