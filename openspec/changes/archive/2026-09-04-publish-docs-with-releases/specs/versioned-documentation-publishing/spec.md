## ADDED Requirements

### Requirement: Releasing a stable version publishes its documentation

Publishing a release SHALL complete documentation validation for the released tag before the
release is published, and SHALL publish documentation for a stable release from that tag without a
supplied version.

#### Scenario: Publish a stable release

- **WHEN** a maintainer publishes a stable release from tag `v2.3.1`
- **THEN** the release SHALL be published and documentation from that tag SHALL be published as
  version `2.3`

#### Scenario: Invalid documentation blocks the release

- **WHEN** the documentation at the selected release tag fails validation
- **THEN** publishing SHALL fail before the release is created and before any published
  documentation version changes

#### Scenario: Publish a pre-release

- **WHEN** a maintainer publishes a release from a pre-release tag
- **THEN** the documentation at that tag SHALL be validated and the published documentation
  versions SHALL remain unchanged

#### Scenario: Recover a failed documentation deployment

- **WHEN** GitHub Pages deployment fails after the release is published
- **THEN** the release SHALL remain published and publishing the same tag through the
  documentation workflow SHALL produce the same documentation version

### Requirement: Release line changes are validated before publication

Documentation validation SHALL run for a proposed change that targets a release line, so that the
documentation of a release line is publishable when it changes.

#### Scenario: Validate a release line correction

- **WHEN** a pull request targets a branch under `release/`
- **THEN** the repository's user documentation validation SHALL run for that pull request

## MODIFIED Requirements

### Requirement: Maintainers can manually publish documentation from a selected source

The documentation workflow SHALL allow a maintainer to select an existing Git tag, an existing
branch whose name begins with `release/`, or a full 40-character commit SHA as the documentation
source, and publish it under an independently selected documentation version. A documentation
version SHALL be a release line such as `2.2` or a full version such as `2.2.0-rc1`. When the
version is omitted, the workflow SHALL derive it from a source name that is `v<version>` or ends
in `/v<version>`, deriving the release line of a stable version and the full version of a
pre-release.

#### Scenario: Publish a release line from its release branch

- **WHEN** a maintainer publishes documentation from a branch such as `release/v2.3` without
  supplying a version
- **THEN** version `2.3` SHALL be published from the current tip of that branch

#### Scenario: Revise a published release line

- **WHEN** a maintainer adds commits to the release branch of an already published release line
  and publishes that branch again
- **THEN** that version SHALL be replaced from the new branch tip while every other published
  version and every release tag SHALL remain unchanged

#### Scenario: Publish a release line derived from a stable tag

- **WHEN** a maintainer publishes documentation from a stable tag such as `v2.3.1` without
  supplying a version
- **THEN** version `2.3` SHALL be published

#### Scenario: Publish a release-candidate version derived from a namespaced tag

- **WHEN** a maintainer publishes documentation from a tag such as `docs/v2.3.0-rc1` without
  supplying a version
- **THEN** version `2.3.0-rc1` SHALL be published

#### Scenario: Publish an explicitly mapped version

- **WHEN** a maintainer publishes a source revision and supplies version `2.2`
- **THEN** the selected source content SHALL be published as version `2.2` regardless of the
  source name

#### Scenario: Publish a full stable version

- **WHEN** a maintainer publishes a stable tag such as `v2.3.0` and supplies version `2.3.0`
- **THEN** version `2.3.0` SHALL be published

#### Scenario: Require an explicit version for an underivable source

- **WHEN** a maintainer publishes a full commit SHA or a source name that does not end in
  `v<version>` without supplying a version
- **THEN** publication SHALL fail before changing any published version and explain that a version
  is required

#### Scenario: Reject a branch outside the release namespace

- **WHEN** a maintainer selects a branch whose name does not begin with `release/`
- **THEN** publication SHALL fail before changing any published version and explain that a branch
  source must be a release branch

#### Scenario: Reject an ambiguous source name

- **WHEN** a maintainer selects a source name that exists as both a branch and a tag
- **THEN** publication SHALL fail before changing any published version and explain that the
  source must be selected as either a branch or a tag

#### Scenario: Reject an unknown source

- **WHEN** a maintainer selects a source name that is neither an existing tag, an existing branch,
  nor the checked-out commit
- **THEN** publication SHALL fail before changing any published version

#### Scenario: Republish one version

- **WHEN** a maintainer publishes a documentation version that already exists
- **THEN** that version SHALL be replaced from the selected source and every other published
  version SHALL remain unchanged
