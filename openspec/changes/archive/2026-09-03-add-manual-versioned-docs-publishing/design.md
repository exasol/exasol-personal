## Context

The repository has no GitHub Pages site, `gh-pages` branch, MkDocs configuration, or
documentation publishing workflow. SPOT-32456 establishes that mechanism independently of the
README migration and release integration that follow in SPOT-32457 and SPOT-32458.

The repository is public and treats workflows with write permissions as privileged automation.
The design therefore keeps the manual trigger on the trusted default branch, pins workflow
dependencies, validates release-style tags, and separates repository mutation from Pages
deployment permissions.

## Goals / Non-Goals

**Goals:**

- Publish, republish, or delete one selected documentation version through one manual workflow.
- Preserve previously published versions and keep `latest` and the site root on the newest
  explicitly published stable version.
- Validate documentation before mutating published state.
- Keep the mechanism small, locally understandable, and isolated from product releases and the
  eventual documentation migration.

**Non-Goals:**

- Migrating or restructuring README content.
- Calling documentation publication from the release workflow.
- Publishing a continuously updated development version.
- Adding a custom domain or integrating with another documentation platform.

## Decisions

### D1. One manual workflow with one operation and one target

The workflow exposes `operation` (`publish` or `delete`) and `target` inputs. For publication,
`target` is an existing semantic-version Git tag such as `v2.3.0` or `v2.3.0-rc1`. For deletion,
it is an existing mike version such as `2.3.0-rc1`.

One workflow keeps concurrency and permissions in one place. Separate publish and delete scripts
or workflows would add indirection before any behavior is shared with the release process. The
workflow remains manual-only in this story; reusable invocation is introduced with release
integration.

### D2. The selected tag supplies the site and dependency lock

Publication checks out the exact selected tag and installs the locked documentation dependencies
stored at that tag. This makes a republish use the same content, configuration, and tool versions
as the original publication. Deletion uses the workflow's trusted default-branch configuration
because it does not build source documentation.

The site content, MkDocs configuration, dependency input, dependency lock, and local version
management live together under the user-documentation directory. The lock is dedicated to
documentation tooling rather than extending the integration test Poetry project. This preserves
the existing separation between product tests and static-site generation.

### D3. mike owns version state on `gh-pages`

mike creates one directory per full version after removing the tag's leading `v`. Stable versions
move the `latest` alias with `--update-aliases` and reset the site root to `latest`. Pre-release
versions remain selectable without changing either pointer. All mike mutations are committed
locally and pushed once, so readers never see half of a multi-command update.

Aliases use redirects rather than symbolic links. This keeps the generated branch portable and
compatible with GitHub Pages publication.

Deletion removes only the requested version. The workflow revalidates the `latest` default before
pushing, so deleting the stable version currently carrying `latest` fails without changing the
remote branch. A newer stable version must be published first.

### D4. Branch state and public deployment are separate jobs

The mutation job receives `contents: write`, updates `gh-pages`, and uploads the resulting static
tree as a Pages artifact. A dependent deployment job receives only `pages: write` and
`id-token: write` and deploys that artifact through GitHub's Pages action. Workflow-level
permissions remain empty, and every external action is pinned to a full commit SHA. Both jobs use
the `github-pages` environment, whose deployment policy admits only workflow runs from `main`.

This uses mike's branch as the durable version store while using GitHub's supported Pages
deployment path. It avoids a personal access token and avoids relying on a workflow-token push to
trigger another workflow.

### D5. Strict MkDocs builds provide the initial link gate

`mkdocs build --strict` is the single pre-publication validation command. It fails on invalid
navigation and unresolved internal documentation links without introducing an external-link
crawler whose network results could make publication nondeterministic. Story SPOT-32457 can add
content-specific validation if the migrated documentation reveals a need.

The repository exposes the same command through a small Task target for local use. The initial
site contains only a minimal placeholder page needed to exercise the publishing mechanism.

### D6. One non-cancelling concurrency group protects version metadata

All publish and delete runs share one concurrency group with cancellation disabled and the maximum
pending queue enabled. Runs execute in request order instead of replacing or interrupting each
other, preventing concurrent updates from losing `versions.json` or an independently published
version.

## Risks / Trade-offs

- **Pages deployment can fail after `gh-pages` was updated** → Keep the branch as the recoverable
  source of truth; rerunning the same operation redeploys the complete generated tree.
- **A stale local view of `gh-pages` could overwrite another run** → Serialize runs and fetch the
  remote publishing branch before every mike operation.
- **Manual input could select an arbitrary ref** → Accept only full semantic-version tags and
  pass the normalized exact `refs/tags/<target>` ref to the pinned checkout action.
- **Deleting the current stable default could break the site root** → Validate `latest` after the
  local deletion and abort before the single remote push if it no longer exists.
- **Pinned Python dependencies require maintenance** → Keep one documentation-only lock and update
  it deliberately in reviewed changes.

## Migration Plan

1. Merge the publishing foundation into `main`.
2. Configure repository Pages settings to use GitHub Actions.
3. Create or select a release-style test tag containing the publishing foundation.
4. Manually publish that tag and verify its version URL, selector, `latest` behavior when stable,
   and root redirect.
5. Manually delete a disposable pre-release version and verify other versions remain available.

Rollback consists of disabling Pages and removing the workflow in a reviewed change. Generated
content remains recoverable from `gh-pages` until that branch is explicitly removed.

## Open Questions

None.
