# versioned-documentation-publishing Specification

## Purpose

Define how maintainers safely validate and manually publish, replace, or delete versioned user
documentation through GitHub Pages.

## Requirements
### Requirement: Maintainers can manually publish documentation from a version tag

The documentation workflow SHALL allow a maintainer to select an existing semantic-version Git
tag and publish the documentation stored at that exact tag under the corresponding version.

#### Scenario: Publish a stable version

- **WHEN** a maintainer publishes documentation from a stable tag such as `v2.3.0`
- **THEN** version `2.3.0` SHALL be published, `latest` SHALL resolve to `2.3.0`, and the site root
  SHALL resolve to `latest`

#### Scenario: Publish a release-candidate version

- **WHEN** a maintainer publishes documentation from a pre-release tag such as `v2.3.0-rc1`
- **THEN** version `2.3.0-rc1` SHALL be published and the existing `latest` alias and site root
  SHALL remain unchanged

#### Scenario: Republish one version

- **WHEN** a maintainer publishes a tag whose documentation version already exists
- **THEN** that version SHALL be replaced from the selected tag and every other published version
  SHALL remain unchanged

### Requirement: Maintainers can manually delete one published version

The documentation workflow SHALL allow a maintainer to select and delete one published version
while retaining the remaining versions and a valid stable default.

#### Scenario: Delete a non-default version

- **WHEN** a maintainer deletes a published version that does not carry the `latest` alias
- **THEN** that version SHALL be removed and every other version, `latest`, and the site root SHALL
  remain unchanged

#### Scenario: Reject deletion of the current stable default

- **WHEN** a maintainer selects the version that currently carries the `latest` alias for deletion
- **THEN** publication SHALL fail before changing the published site and explain that a newer
  stable version must be published first

### Requirement: Published versions are discoverable from the site

Every published page SHALL expose version navigation containing all retained documentation
versions.

#### Scenario: Navigate between retained versions

- **WHEN** a reader opens a published documentation page and selects another retained version
- **THEN** the site SHALL navigate to the corresponding page in that version

### Requirement: Documentation is validated before publication

The workflow SHALL complete a strict documentation build and internal-link validation before it
changes published version state.

#### Scenario: Validation succeeds

- **WHEN** the selected tag contains valid documentation and internal links
- **THEN** the workflow SHALL proceed with the requested publication

#### Scenario: Validation fails

- **WHEN** the selected tag contains invalid documentation configuration, navigation, or internal
  links
- **THEN** the workflow SHALL fail without changing any published version

### Requirement: Publication operations preserve a consistent version catalog

Publish and delete operations SHALL execute serially and deploy the complete resulting version
catalog.

#### Scenario: Two operations are requested concurrently

- **WHEN** another publication operation is already running
- **THEN** the later operation SHALL wait and then apply its change to the version catalog produced
  by the earlier operation

#### Scenario: Pages deployment fails after version state is stored

- **WHEN** GitHub Pages deployment fails after the version catalog has been updated
- **THEN** rerunning the same manual operation SHALL deploy the complete stored version catalog
