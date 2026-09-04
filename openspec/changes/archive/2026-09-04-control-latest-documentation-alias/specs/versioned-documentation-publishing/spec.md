## ADDED Requirements

### Requirement: The latest alias resolves to the highest published stable version

Publication SHALL grant the `latest` alias and the site root to the requested stable version only
when no higher stable version is published. A publication SHALL accept an explicit alias decision
that overrides this comparison, and SHALL reject granting `latest` to a pre-release version.

#### Scenario: Publish the highest stable version

- **WHEN** a maintainer publishes stable version `2.3.0` while `2.2.0` is the highest published
  stable version
- **THEN** `latest` and the site root SHALL resolve to `2.3.0`

#### Scenario: Publish documentation for an older release

- **WHEN** a maintainer publishes stable version `2.1.1` while `2.2.0` is the highest published
  stable version
- **THEN** `2.1.1` SHALL be published and `latest` and the site root SHALL continue to resolve to
  `2.2.0`

#### Scenario: Publish a pre-release version

- **WHEN** a maintainer publishes a pre-release version
- **THEN** that version SHALL be published and `latest` and the site root SHALL remain unchanged

#### Scenario: Force the alias onto a published version

- **WHEN** a maintainer publishes a stable version and requests that it carry `latest`
- **THEN** `latest` and the site root SHALL resolve to that version regardless of the published
  version order

#### Scenario: Withhold the alias from the highest stable version

- **WHEN** a maintainer publishes the highest stable version and requests that it not carry
  `latest`
- **THEN** that version SHALL be published and `latest` and the site root SHALL remain unchanged

#### Scenario: Reject forcing the alias onto a pre-release

- **WHEN** a maintainer publishes a pre-release version and requests that it carry `latest`
- **THEN** publication SHALL fail before changing any published version and explain that only a
  stable version can carry `latest`

## MODIFIED Requirements

### Requirement: Published versions are discoverable from the site

Every published page SHALL expose version navigation containing all retained documentation
versions and identifying the version that carries the `latest` alias.

#### Scenario: Navigate between retained versions

- **WHEN** a reader opens a published documentation page and selects another retained version
- **THEN** the site SHALL navigate to the corresponding page in that version

#### Scenario: Recognize the current version

- **WHEN** a reader opens a page of a version published with the `latest` alias
- **THEN** version navigation SHALL show that the selected version is `latest`

### Requirement: Maintainers can manually publish documentation from an immutable source revision

The documentation workflow SHALL allow a maintainer to select an existing Git tag or full commit
SHA as the documentation source and publish it under an independently selected semantic version.
When the version is omitted, the workflow SHALL derive it from a tag named `v<semver>` or ending
in `/v<semver>`.

#### Scenario: Publish a stable version derived from a tag

- **WHEN** a maintainer publishes documentation from a tag such as `v2.3.0` without supplying a
  version
- **THEN** version `2.3.0` SHALL be published

#### Scenario: Publish a release-candidate version derived from a namespaced tag

- **WHEN** a maintainer publishes documentation from a tag such as `docs/v2.3.0-rc1` without
  supplying a version
- **THEN** version `2.3.0-rc1` SHALL be published

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
