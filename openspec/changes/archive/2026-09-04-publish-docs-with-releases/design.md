## Context

Two manual workflows exist. The release workflow is dispatched with an existing tag and runs
GoReleaser against it. The documentation workflow is dispatched with a source revision and
publishes one documentation version to GitHub Pages. Nothing connects them, so documentation
alignment depends on a maintainer remembering a second dispatch.

The release line model established for documentation sources publishes one version per minor
line, such as `2.2` from `release/v2.2`. A release line branch is created from the first stable
tag of its line, which means it does not exist while that release is being published.

## Decisions

### Release line versions derive from stable tags

A stable tag derives the release line it belongs to, so `v2.3.0` and `v2.3.1` both derive `2.3`.
Pre-release tags keep their full version, because a pre-release documents one candidate rather
than a line. An explicit version still overrides derivation, which retains mapping historical
content onto an older line.

The alternative is a release workflow that computes the line and passes it as an explicit version.
That leaves two rules for one question: manually publishing the same tag would derive a full
version and produce a different published version than the release did. Since the manual workflow
is the recovery path for the automated one, recovery has to reproduce what it recovers, so the
rule belongs in the shared version helper rather than in the release workflow.

Deriving from the tag also removes the ordering problem. The release publishes its line from the
released commit, so no release line branch is required while releasing, and the branch is needed
only for later corrections.

### Documentation validation gates the release

Validation runs as a separate job before GoReleaser, using the content, configuration, scripts,
and locked dependencies of the tag being released. It performs the same strict build the
publication path performs and rejects a request whose version cannot be derived, so a release
cannot reach a state where its documentation cannot be published afterwards.

Publication runs after the release is published. A failure there leaves the release in place,
because withdrawing a signed and notarized release to correct a documentation link is a worse
outcome than a short gap in published documentation. The maintainer recovers by dispatching the
documentation workflow for the same tag, which derives the same version.

Pre-releases are validated and publish nothing. Publishing every candidate would add versions that
only an explicit deletion removes, while validating them keeps a documentation break from reaching
the stable release unnoticed.

### One publication implementation, two entry points

The documentation workflow accepts a workflow call in addition to its manual dispatch, and the
release workflow calls it. A second implementation would drift from the recovery path it is meant
to mirror. The shared workflow keeps one serialized concurrency group, so a release publication
and a manual publication cannot interleave.

A called workflow runs under the calling run's ref, so the release workflow has to be dispatched
from the default branch for the `github-pages` environment to accept the deployment. That is
already how releases are dispatched, since the workflow checks out the tag it is given and does
not inspect the selected branch.

### Release lines are validated but created manually

Continuous integration accepts pull requests targeting `release/*`, so a documentation correction
or a patch on a release line is validated before that line is published again. Creating the line
branch stays a maintainer step, because it is a release-process decision about which commits a
line carries and it is not on the critical path for publication.

## Risks / Trade-offs

- A tag without a complete documentation setup blocks its release. Mitigation: continuous
  integration validates the same setup on every pull request to `main` and to a release line, so
  the gate fails only for a tag that was never validated.
- Documentation publication and the release are no longer independent, so a Pages outage delays
  documentation for a published release. Mitigation: the release is unaffected and the manual
  workflow republishes the same tag to the same version.
- Stable tags no longer derive a full version. Mitigation: an explicit version input still
  publishes one, and no published version depends on the previous derivation.
