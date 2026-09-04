## Why

Releasing a version and publishing its documentation are two unconnected manual workflows, so a
release can ship with documentation that is missing, stale, or invalid. The documented release
process also still describes a tag push as the release trigger, which stopped being true when
releases moved to an explicit manual dispatch.

## What Changes

- Publish documentation for a stable release from the release workflow, using the release tag as
  the source and requiring no version input.
- **BREAKING** Derive a release line such as `2.3` from a stable release tag, instead of the full
  version `2.3.0`. Pre-release tags keep their full version, and an explicit version still
  overrides derivation.
- Complete documentation validation before a release is published, so invalid documentation
  prevents the release.
- Validate the documentation of a pre-release without publishing a documentation version.
- Validate changes proposed to a release line, so a release line is publishable when its
  documentation changes.
- Describe in the release and CI guides which release steps a maintainer performs and which the
  release workflow performs.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `versioned-documentation-publishing`: Releasing a stable version publishes its release line
  documentation, and a stable tag derives a release line version.

## Impact

The release workflow, the documentation workflow, the version helper script and its tests, the CI
pull request triggers, the release guide, the CI guide, and the versioned documentation publishing
specification change. Releasing gains a documentation validation gate. No product behavior and no
additional dependency is introduced.
