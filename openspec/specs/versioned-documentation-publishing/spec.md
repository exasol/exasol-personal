# versioned-documentation-publishing Specification

## Purpose

Define how maintainers safely validate and manually publish, replace, or delete versioned user
documentation through GitHub Pages.
## Requirements
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

The workflow SHALL check out the selected source as an exact commit and complete a strict
documentation build and internal-link validation using the content, configuration, scripts, and
locked dependencies stored at that revision before it changes published version state.

#### Scenario: Validation succeeds

- **WHEN** the selected commit contains a valid self-contained `user-docs/` setup
- **THEN** the workflow SHALL proceed with the requested publication using the exact resolved
  commit

#### Scenario: Selected source lacks a complete documentation setup

- **WHEN** the selected commit lacks required documentation content, configuration, scripts, or
  locked dependencies
- **THEN** the workflow SHALL fail without changing any published version

#### Scenario: Validation fails

- **WHEN** the selected documentation snapshot has invalid navigation or internal links
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

### Requirement: Maintainers can manually publish documentation from an immutable source revision

The documentation workflow SHALL allow a maintainer to select an existing Git tag or full commit
SHA as the documentation source and publish it under an independently selected semantic version.
When the version is omitted, the workflow SHALL derive it from a tag named `v<semver>` or ending
in `/v<semver>`.

#### Scenario: Publish a stable version derived from a tag

- **WHEN** a maintainer publishes documentation from a tag such as `v2.3.0` without supplying a
  version
- **THEN** version `2.3.0` SHALL be published, `latest` SHALL resolve to `2.3.0`, and the site root
  SHALL resolve to `latest`

#### Scenario: Publish a release-candidate version derived from a namespaced tag

- **WHEN** a maintainer publishes documentation from a tag such as `docs/v2.3.0-rc1` without
  supplying a version
- **THEN** version `2.3.0-rc1` SHALL be published and the existing `latest` alias and site root
  SHALL remain unchanged

#### Scenario: Publish an explicitly mapped version

- **WHEN** a maintainer publishes an immutable source revision and supplies version `2.2.0`
- **THEN** the selected source content SHALL be published as version `2.2.0` regardless of the
  source tag name

#### Scenario: Require an explicit version for an underivable source

- **WHEN** a maintainer publishes a full commit SHA or a tag that does not end in `v<semver>`
  without supplying a version
- **THEN** publication SHALL fail before changing any published version and explain that a version
  is required

#### Scenario: Reject a mutable source

- **WHEN** a maintainer selects a branch as the documentation source
- **THEN** publication SHALL fail before changing any published version and explain that only an
  exact tag or full commit SHA is accepted

#### Scenario: Republish one version

- **WHEN** a maintainer publishes a documentation version that already exists
- **THEN** that version SHALL be replaced from the selected source and every other published
  version SHALL remain unchanged
