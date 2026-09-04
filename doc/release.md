# Release Process

This document describes how to create and publish releases of Exasol Personal.

Security requirements for release automation are defined in [Repository Security and Automation Governance](repository_security_spec.md).

## Overview

A release is built, signed, and published by GitHub Actions, but it is not triggered by a tag push.
A maintainer pushes a version tag and dispatches the release workflow for that tag. The
[CI guide](ci.md#release-pipeline-releaseyml) describes the workflow itself.

The maintainer:
- Finalizes the changelog and commits it.
- Creates and pushes the version tag.
- Dispatches the release workflow for that tag.
- Creates the release line branch after the first stable release of a minor version.

The release workflow:
- Validates the user documentation at the release tag.
- Builds, signs, and notarizes binaries for all supported platforms with [GoReleaser](https://goreleaser.com/).
- Creates the GitHub release with its artifacts, checksums, and generated notes.
- Publishes the user documentation of a stable release as that release's line version.

Tests are not part of the release workflow. They run in CI on the commit before it is tagged.

Release safety gates:
- Version tags must follow `v*`.
- Invalid user documentation at the release tag fails the release before it is published.
- Publishing and signing run in a protected release environment.
- Third-party release actions are pinned to immutable commit SHAs.
- Downloaded signing tooling is version-pinned and checksum-verified in CI.

Tag governance controls (for example restricting who can create `v*` tags and what refs are allowed) are enforced through repository rulesets/settings.

## Release Lines

Each minor version has a release line branch named `release/v<major>.<minor>`, created from that
line's first stable release tag. A release publishes its documentation from the release tag, so the
branch is not needed in order to release. It exists to carry later corrections: content and styling
can be fixed after the release without moving a release tag, and a fix is reviewed together with the
documentation it changes. Republishing a corrected line is described in the
[CI guide](ci.md#documentation-publication-docsyml).

Tagging is unchanged on a release line: the release workflow checks out the tag it is given and
does not inspect branches, so a patch is tagged on its release branch exactly as a release is
tagged on the default branch. Dispatch the release workflow with the default branch selected, so
that documentation publication is accepted by the `github-pages` environment. Protect `release/*`
with a ruleset, as described in [CI security best practices](ci_security_best_practices.md).

Which changes reach a release line is decided per line and is not automated.

## Creating a Release

Before tagging a release, ensure deployment directory compatibility constraints are up to date:

- If the release introduces a breaking change in deployment directory semantics (state layout, workflow invariants, marker files, etc.), add a new minimum supported deployment version constant in the cmd layer (see `cmd/exasol/compatibility_versions.go`) and apply it to the affected commands.
- Release-candidate versions (for example `1.2.0-rc1`) must not appear in those constants. Compatibility comparisons treat prerelease/build suffixes as irrelevant and compare only the base version (so `1.2.0-rc1` behaves like `1.2.0`).

Before tagging a stable release, finalize the user-facing [changelog](../CHANGELOG.md). The in-repo changelog is the durable release history for users; generated GitHub release notes are useful for publishing, but they are not a substitute for maintaining the curated changelog.

Move all applicable entries from `Unreleased` into a new versioned section such as `2.2.0 - 2026-07-16`, grouped by `Added`, `Changed`, `Fixed`, and `Breaking Changes`. Commit that release-prep change first, then create the stable release tag on that commit. Automating this step is desirable, but the manual release-prep commit is the required fallback.

Pre-release tags such as `v2.2.0-rc1` do not require this extra changelog-finalization commit. Their entries may remain under `Unreleased` until the stable release is prepared.

### 1. Finalize the Changelog

```bash
# Edit CHANGELOG.md:
# - create the version section for the release
# - move all shipped entries from Unreleased into that section
# - leave Unreleased ready for the next cycle
git add CHANGELOG.md
git commit -m "docs: finalize changelog for v1.2.3"
```

### 2. Tag the Release

```bash
# Create an annotated tag with semantic versioning
git tag -a v1.2.3 -m "Release v1.2.3"
git push origin v1.2.3
```

### 3. Publish the Release

Dispatch [Create GH release (from existing tag)](https://github.com/exasol/exasol-personal/actions/workflows/release.yml)
with the default branch selected and the tag as its input. Leave the previous tag blank unless the
changelog base has to be something other than the last stable release.

Watch the run to completion. It validates the tag's documentation, waits for the release
environment's approval, publishes the release, and then publishes the documentation of a stable
release as version `<major>.<minor>`. A pre-release validates its documentation and publishes no
documentation version.

If documentation publication fails after the release is published, the release stays published.
Republish the documentation by dispatching the
[documentation workflow](ci.md#documentation-publication-docsyml) for the same tag, which resolves
the same version.

### 4. Create the Release Line Branch

After the first stable release of a minor version, create that line's branch so later corrections
have a home:

```bash
git branch release/v1.2 v1.2.0
git push origin release/v1.2
```

## Release Configuration

The release process is configured in `.goreleaser.yaml`, which defines:

- **Build matrix**: OS and architecture combinations
- **Binary naming**: Naming conventions for executables
- **Binary size policy**: Raw binary optimization flags documented in [Binary Size Optimization](binary_size.md)
- **Archives**: Packaging format (tar.gz, zip)
- **Checksums**: SHA256 checksums for verification
- **Release notes**: Automatically generated from commits

## Supported Platforms

Releases are built for:
- **Linux**: amd64, arm64
- **macOS**: amd64 (Intel), arm64 (Apple Silicon)
- **Windows**: amd64

## Testing Releases Locally

To test the release process without publishing:

```bash
# Requires GoReleaser installed
goreleaser release --snapshot --clean
```

This creates a local build in the `dist/` directory without creating a GitHub release.

## Versioning

Follow [Semantic Versioning](https://semver.org/):
- **Major (v1.0.0)**: Breaking changes
- **Minor (v1.1.0)**: New features, backwards compatible
- **Patch (v1.0.1)**: Bug fixes, backwards compatible

## Release Checklist

Before creating a stable release:

- [ ] All changes merged to `main`, or to the release line for a patch release
- [ ] CI passed on the commit to be tagged
- [ ] User documentation under `user-docs/` describes this release
- [ ] Changelog is finalized for this version and committed before the tag
- [ ] Version number follows semantic versioning
- [ ] Tag created with proper version format (`v1.2.3`)

After the release workflow completes:

- [ ] Published documentation shows the released version
- [ ] Release line branch exists for the version
