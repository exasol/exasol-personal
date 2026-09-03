## Context

The manual workflow currently treats a release tag as both the source revision and published version. That prevents retroactive publication because immutable historical release tags do not contain the new documentation structure or tooling. A later documentation snapshot based on a current commit can contain the historical content and complete publishing environment without changing the original release tag.

## Goals / Non-Goals

**Goals:**

- Select a Git tag or full commit SHA independently from the published semantic version.
- Publish a self-contained documentation snapshot with one shallow checkout.
- Keep source and version validation locally testable with the catalog operations.

**Non-Goals:**

- Converting historical README content into the normalized documentation tree.
- Accepting mutable branch references.
- Integrating publication into the release workflow.

## Decisions

### D1. Publication has independent `source_ref` and `version` inputs

`source_ref` identifies either an exact tag or a full 40-character commit SHA. An explicit `version` is authoritative; otherwise the version is derived from a tag named `v<semver>` or whose final path component is `v<semver>`. Delete operations use only the required `version` input.

Separate inputs make retroactive mappings explicit without adding another workflow or moving release tags. Full SHAs and exact tags provide immutable source selection; branches remain outside the contract.

### D2. The selected revision is a self-contained documentation snapshot

Publication checks out the selected tag or commit once with shallow history. Its `user-docs/` directory supplies the content, MkDocs configuration, scripts, and locked uv environment used for the build and catalog operation.

Keeping the snapshot complete makes it directly reviewable and reproducible. A retroactive tag such as `docs/v2.2.0` can point to a recent commit containing current tooling and the prepared historical content.

### D3. The workflow validates the checked-out revision before running it

Before invoking repository scripts, the workflow verifies that a full SHA matches the checked-out commit or that an exact tag resolves to it. Branch references therefore fail without executing their content.

This retains the immutable-source contract without a second checkout, archive transport, or content-copying layer. Deletion selects `main` because it has no documentation source.

### D4. Version catalog behavior remains version-based

After strict validation, the existing mike operations publish the normalized version. Stable versions update `latest`; prereleases do not. Deletion remains independent of source content and accepts only a version.

## Risks / Trade-offs

- **A selected revision lacks a complete documentation environment** → Fail during setup or validation before changing the version catalog.
- **A selected snapshot contains unreviewed tooling** → Require maintainers to publish only reviewed immutable tags or commits and validate the ref before executing repository code.
- **A tag and explicit version describe different releases** → Treat the explicit mapping as intentional and show both the resolved commit and version in the workflow summary.
- **Snapshot tooling eventually becomes stale** → Base retroactive snapshots on a current reviewed commit rather than on the original release tag.

## Migration Plan

Replace the manual workflow inputs and keep all existing published versions unchanged. Existing release tags continue to work when they contain the complete `user-docs/` setup; a reviewed documentation snapshot based on a current commit can supply an older published version. Reverting the workflow changes restores the previous input contract without changing `gh-pages` history.

## Open Questions

None.
