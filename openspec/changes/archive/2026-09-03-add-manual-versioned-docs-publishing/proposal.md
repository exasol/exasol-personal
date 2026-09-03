## Why

Exasol Personal needs a release-independent way to validate and publish versioned user
documentation before that mechanism is coupled to the release process. A manually dispatched
workflow provides a small, testable foundation for publishing, republishing, and removing
documentation versions safely.

## What Changes

- Add a minimal Material for MkDocs site configured for version navigation with mike.
- Add a manually dispatched workflow that publishes documentation from a selected existing Git
  tag or deletes a selected published version.
- Retain every published version independently, update `latest` and the site root for stable
  versions, and keep release-candidate versions selectable without changing `latest`.
- Validate the documentation build and its links before changing published content.
- Serialize publication operations and give each job only the permissions it requires.
- Deploy the generated versioned site through GitHub Pages.

## Capabilities

### New Capabilities

- `versioned-documentation-publishing`: Manual validation, version management, deletion, and
  GitHub Pages publication of documentation built from repository tags.

### Modified Capabilities

None.

## Impact

- Adds documentation configuration, a small placeholder page, and locked Python documentation
  dependencies.
- Adds a privileged manual GitHub Actions workflow and a generated `gh-pages` branch.
- Requires GitHub Pages to use GitHub Actions as its publishing source.
- Leaves README migration and release-workflow integration to their dedicated follow-up stories.
