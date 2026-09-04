## Why

Content and styling of documentation for an already released version need to change after that
release. Publication currently accepts only immutable sources, and a release tag cannot move, so a
revision has no source to publish from.

## What Changes

- Accept a branch under `docs/` as a documentation source alongside tags and commit SHAs.
- Reject branches outside that namespace, so publication cannot be requested from a development
  branch.
- Derive the published version from a documentation branch ending in `v<semver>`, matching the
  existing derivation from tags.
- Reject a source name that exists as both a branch and a tag.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `versioned-documentation-publishing`: Publish from a revisable documentation branch within a
  reserved namespace.

## Impact

The manual documentation workflow, its helper script and tests, the CI guide, and the versioned
documentation publishing specification change. No runtime product behavior or additional
dependency is introduced.
