# CI Security Specification for Open-Sourcing

Status: Draft for review before implementation.

This specification defines mandatory CI/CD security controls for this repository as it becomes public. General guidance is in [CI Security Best Practices](ci_security_best_practices.md); this document defines repo-specific requirements.

## Security Objectives

- External contributors can run basic CI checks through pull requests.
- Only trusted contributors can trigger privileged workflows in this repository.
- Release automation runs only for protected version tags that reference commits in `main` history.
- Supply-chain risk in workflows is reduced through pinning, allowlists, and verification.

## Contribution Trust Model

- Public contribution happens through fork-based pull requests.
- Untrusted PR code is allowed to run only in the untrusted CI lane.
- The untrusted lane must not receive repository/environment secrets and must not receive cloud credentials.
- Privileged workflows (deployment, release, signing, cloud access) are restricted to trusted triggers.

This model is safe because a fork PR can change workflow code, but in the untrusted lane there are no high-value credentials to exfiltrate and no write-capable token scopes.

## Repository Configuration Requirements

### Actions Settings (MUST)

- Actions policy must allow only:
  - local actions from this repository, and
  - a curated allowlist of external actions required by this project.
- Actions policy must require full-length commit SHA pinning for external actions.
- Default `GITHUB_TOKEN` permissions must be restricted.
- "Allow GitHub Actions to create and approve pull requests" must remain disabled.
- "Send write tokens to workflows from pull requests" must remain disabled.
- "Send secrets to workflows from pull requests" must remain disabled.

### Branch and Tag Governance (MUST)

- A ruleset for `main` must enforce:
  - pull-request-based changes,
  - required status checks for CI,
  - required reviews including code-owner review,
  - dismissal of stale approvals after new commits,
  - no force-push and no deletion.
- A ruleset for release tags (`v*`) must restrict create/update/delete to release maintainers only.
- Bypass permissions for rulesets must be minimal and explicitly documented.

### Workflow Ownership (MUST)

- `CODEOWNERS` must include:
  - `.github/workflows/**`
  - `.github/actions/**`
  - this specification document
- Code owner review must be required for any change to those paths.

### Release Environment Controls (MUST)

- Release publication/signing must use a protected environment (for example `release`) with required reviewers.
- Environment deployment branch/tag restrictions must allow only intended release refs.

## Workflow Design Requirements

### Untrusted Pull Request Lane (MUST)

- Only [`ci.yml`](/home/nh/ws/exasol-personal/.github/workflows/ci.yml) may trigger on pull requests.
- `ci.yml` pull request jobs are limited to basic CI scope: lint, build, unit tests, and other non-privileged checks explicitly approved by maintainers.
- `ci.yml` must not request `id-token: write` and must not access repository/environment secrets.
- `ci.yml` must use explicit minimal permissions for PR jobs.
- `pull_request_target` must not be used for executing contributor code.

### Trusted Lane for Privileged Workflows (MUST)

- Privileged workflows must use only:
  - `push` on trusted branches/tags, and
  - `workflow_dispatch` for trusted contributors.
- Privileged workflows must not run on `pull_request` events.
- Each privileged workflow must set explicit minimal `permissions`.
- Write scopes (for example `contents: write`) are allowed only in dedicated publish/release jobs.

### Release Workflow Constraints (MUST)

- Release workflow trigger remains version tags (`v*`) only.
- Workflow must fail before any publish/sign step unless the tagged commit is contained in `main` history.
- Release workflow must run with elevated permissions only after ref-validation gates pass.

### Manual Workflow Constraints (MUST)

- Manually-triggered privileged workflows must validate ref context and fail for non-approved refs.
- Manual deployment-style workflows must require environment approval before cloud access or publication.

## Supply-Chain Hardening Requirements

- External `uses:` references must be pinned to full commit SHA.
- Downloaded tools in workflows/composite actions must use fixed versions and checksum/signature verification.
- Mutable install patterns (for example bare `curl ... | sh` without version/checksum controls) must be removed from privileged workflows.
- Dependabot (or equivalent automation) must manage updates for GitHub Actions dependencies.
- Release artifacts must include verifiable integrity metadata (checksums; attestations where feasible).

## Acceptance Criteria

- A pull request from a fork can run only [`ci.yml`](/home/nh/ws/exasol-personal/.github/workflows/ci.yml) in this repository.
- A fork PR cannot access repository/environment secrets or cloud credentials.
- A user without write access cannot start privileged workflow execution.
- A tag matching `v*` created on a commit outside `main` history cannot produce a release.
- Workflow changes without code owner approval cannot merge to `main`.
- Release tags can be created only by authorized release maintainers.

## Implementation Notes for Follow-Up

- Implementation will be done in a separate change set after this specification is approved.
- Temporary rollout checklist is tracked in [CI Security TODO](ci_security_todo.md).
- `doc/ci.md` and `doc/release.md` should reference this specification once controls are implemented.
