## Why

Documentation branches and release branches describe the same release line. Maintaining both
splits a line across two branches, so a patch and the documentation it changes cannot be reviewed
together and can disagree. A release line also covers every patch it carries, which a version
published per patch cannot express without listing near-identical entries.

## What Changes

- Accept a branch under `release/` as a documentation source, replacing the documentation branch
  namespace.
- Accept a release line such as `2.2` as a published documentation version alongside full
  versions.
- Order the `latest` alias by release line, treating a line and its first patch as the same
  position.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `versioned-documentation-publishing`: Publish a release line from its release branch.

## Impact

The manual documentation workflow, its helper script and tests, the CI guide, and the versioned
documentation publishing specification change. Publishing requires a release branch for the line
being documented. No runtime product behavior or additional dependency is introduced.
