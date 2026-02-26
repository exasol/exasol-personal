# Release Process

This document describes how to create and publish releases of Exasol Personal.

Security requirements for release automation are defined in [Repository Security and Automation Governance](repository_security_spec.md).

## Overview

Releases are fully automated using [GoReleaser](https://goreleaser.com/) and GitHub Actions. When a version tag is pushed, the release workflow automatically:
- Builds binaries for all supported platforms
- Runs the test suite
- Creates a GitHub release
- Uploads release artifacts

Release safety gates:
- Version tags must follow `v*`.
- Publishing and signing run in a protected release environment.
- Third-party release actions are pinned to immutable commit SHAs.
- Downloaded signing tooling is version-pinned and checksum-verified in CI.

Tag governance controls (for example restricting who can create `v*` tags and what refs are allowed) are enforced through repository rulesets/settings.

## Creating a Release

### 1. Tag the Release

```bash
# Create an annotated tag with semantic versioning
git tag -a v1.2.3 -m "Release v1.2.3"

# Push the tag to trigger the release workflow
git push origin v1.2.3
```

### 2. Automated Build

GitHub Actions will automatically:
1. Checkout the tagged commit
2. Run tests to ensure quality
3. Build binaries for all target platforms
4. Create checksums and archives
5. Generate release notes
6. Publish the release on GitHub

### 3. Monitor the Release

Watch the GitHub Actions workflow to ensure it completes successfully:
- Navigate to the [Actions tab](https://github.com/exasol/exasol-personal/actions)
- Find the workflow run for your tag
- Verify all jobs complete successfully

## Release Configuration

The release process is configured in `.goreleaser.yaml`, which defines:

- **Build matrix**: OS and architecture combinations
- **Binary naming**: Naming conventions for executables
- **Archives**: Packaging format (tar.gz, zip)
- **Checksums**: SHA256 checksums for verification
- **Release notes**: Automatically generated from commits

## Supported Platforms

Releases are built for:
- **Linux**: amd64, arm64
- **macOS**: amd64 (Intel), arm64 (Apple Silicon)
- **Windows**: amd64, arm64

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

Before creating a release:

- [ ] All tests pass locally (`task all`)
- [ ] Documentation is up to date
- [ ] Version number follows semantic versioning
- [ ] All changes merged to main branch
- [ ] Tag created with proper version format (`v1.2.3`)
