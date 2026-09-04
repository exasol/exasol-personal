## RENAMED Requirements

- FROM: `### Requirement: Maintainers can manually publish documentation from an immutable source revision`
- TO: `### Requirement: Maintainers can manually publish documentation from a selected source`

## MODIFIED Requirements

### Requirement: Maintainers can manually publish documentation from a selected source

The documentation workflow SHALL allow a maintainer to select an existing Git tag, an existing
branch whose name begins with `docs/`, or a full 40-character commit SHA as the documentation
source, and publish it under an independently selected semantic version. When the version is
omitted, the workflow SHALL derive it from a source name that is `v<semver>` or ends in
`/v<semver>`.

#### Scenario: Publish a stable version derived from a tag

- **WHEN** a maintainer publishes documentation from a tag such as `v2.3.0` without supplying a
  version
- **THEN** version `2.3.0` SHALL be published

#### Scenario: Publish a release-candidate version derived from a namespaced tag

- **WHEN** a maintainer publishes documentation from a tag such as `docs/v2.3.0-rc1` without
  supplying a version
- **THEN** version `2.3.0-rc1` SHALL be published

#### Scenario: Publish from a documentation branch

- **WHEN** a maintainer publishes documentation from a branch such as `docs/v2.3.0` without
  supplying a version
- **THEN** version `2.3.0` SHALL be published from the current tip of that branch

#### Scenario: Revise a published version from its documentation branch

- **WHEN** a maintainer adds commits to the documentation branch of an already published version
  and publishes that branch again
- **THEN** that version SHALL be replaced from the new branch tip while every other published
  version and the release tag of that version SHALL remain unchanged

#### Scenario: Publish an explicitly mapped version

- **WHEN** a maintainer publishes a source revision and supplies version `2.2.0`
- **THEN** the selected source content SHALL be published as version `2.2.0` regardless of the
  source name

#### Scenario: Require an explicit version for an underivable source

- **WHEN** a maintainer publishes a full commit SHA or a source name that does not end in
  `v<semver>` without supplying a version
- **THEN** publication SHALL fail before changing any published version and explain that a version
  is required

#### Scenario: Reject a branch outside the documentation namespace

- **WHEN** a maintainer selects a branch whose name does not begin with `docs/`
- **THEN** publication SHALL fail before changing any published version and explain that a branch
  source must be a documentation branch

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
