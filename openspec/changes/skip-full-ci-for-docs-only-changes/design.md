## Context

The current pull request CI workflow runs the same full validation pipeline for documentation-only changes as it does for source changes. GitHub required checks make top-level workflow skipping and path filters risky because skipped workflows can leave required checks pending.

## Goals / Non-Goals

**Goals:**

- Classify pull request changes by file path inside the workflow.
- Keep the full CI exemption list in a root `.ciignore` file.
- Run documentation quality checks independently from the code validation pipeline.
- Skip expensive code validation jobs only when all changed files are documentation-only.
- Provide one final required status check that works with GitHub branch protection.

**Non-Goals:**

- Detect comment-only changes inside source files.
- Add label-based or branch-name-based skip controls.
- Add Markdown linting or spelling tools as part of this change.
- Change release, deployment-test, or main-branch merge workflows.

## Decisions

- Use a first CI job to classify changed files. This keeps the workflow running so branch protection receives a final status check, while allowing later jobs to skip by job condition.
- Store the full CI exemption list in `.ciignore` as a flat list of gitignore-style glob patterns. Any pull request file outside that allow-list requires full CI.
- Always run full CI for pushes to `main`. The classifier is only allowed to skip full CI for pull requests.
- Keep a dedicated documentation quality stage even before concrete tools are configured. This creates the insertion point for Markdown linting and spelling checks without changing the full-CI gate design later.
- Require a final aggregate job in repository settings. Individual jobs can be intentionally skipped, so branch protection should depend on the aggregate result instead of the skipped jobs.
- Protect `.ciignore` with CODEOWNERS because it controls which pull requests can skip full CI.

## Risks / Trade-offs

- Documentation path rules can become stale as the repository layout changes. Mitigation: keep `.ciignore` small and update it when adding new documentation locations.
- Full CI can still run for harmless source-comment changes. Mitigation: this is intentional for the first version because path-only classification is simple and auditable.
- The final required check depends on repository settings outside version control. Mitigation: document the required setting in CI documentation and the implementation tasks.
