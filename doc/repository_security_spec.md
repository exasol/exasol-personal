# Repository Security and Automation Governance

This document defines required repository settings and workflow design controls for operating this repository securely as a public project.

General guidance is in [CI Security Best Practices](ci_security_best_practices.md). This document is the repository-specific policy and setup guide for admins and maintainers.

## Security Model

- Public pull requests are treated as untrusted execution.
- Trusted automation (release, signing, cloud-backed tests) must run only from trusted refs and approved contexts.
- Security relies on two layers that must stay aligned:
  - in-repo workflow controls (workflow triggers, permissions, pinning, verification), and
  - repository settings controls (Actions policy, rulesets, environments, token defaults, approvals).

## In-Repo Controls (Required)

### Untrusted Pull Request Lane

- Only [`ci.yml`](../.github/workflows/ci.yml) runs on `pull_request`.
- PR jobs are non-privileged and must not use secrets or OIDC cloud credentials.
- PR jobs use explicit minimal permissions (`contents: read`).
- `pull_request_target` is not used for contributor code execution.

### Trusted Privileged Lane

- Privileged workflows run only on trusted events (`push` or `workflow_dispatch`):
  - [`merge.yml`](../.github/workflows/merge.yml)
  - [`release.yml`](../.github/workflows/release.yml)
  - [`tests-deployment.yml`](../.github/workflows/tests-deployment.yml)
- Privileged workflows do not run on `pull_request`.
- Workflows declare explicit `permissions` and only request write scopes where required.

### Release and Supply-Chain Controls

- Release/signing job is gated by protected environment `release`.
- Third-party actions are pinned to full commit SHA.
- Downloaded signing tooling in release is version-pinned and checksum-verified.
- Mutable installer patterns in privileged paths are not allowed.
- GitHub Actions dependencies are maintained through Dependabot.

### Workflow Change Governance

- [`CODEOWNERS`](../.github/CODEOWNERS) protects workflow, action, and release-control paths.
- Security-sensitive automation paths require code-owner review.

## Admin Setup Checklist (GitHub Settings)

These controls are configured in repository/org settings and are mandatory for this repository.

### 1) Actions Policy and Token Defaults

- Configure Actions policy to allow only:
  - local actions from this repository, and
  - an explicit allowlist of required external actions.
- Enable the policy that requires full-length commit SHA pinning for external actions.
- Set default `GITHUB_TOKEN` permissions to restricted.
- Keep these settings disabled:
  - `Allow GitHub Actions to create and approve pull requests`
  - `Send write tokens to workflows from pull requests`
  - `Send secrets to workflows from pull requests`
- Set fork-run approval policy to require maintainer approval before first run.

This enforces the trust boundary expected by the untrusted PR lane in [`ci.yml`](../.github/workflows/ci.yml).

### 2) Branch and Tag Rulesets

- Create and enforce a `main` ruleset with:
  - pull-request-only updates,
  - required CI checks,
  - required reviews including code-owner review,
  - stale approval dismissal on new commits,
  - no force-push and no deletion.
- Create and enforce a `v*` tag ruleset that limits create/update/delete to release maintainers.
- Review and minimize ruleset bypass permissions.

This is the administrative control for release ref governance and merge governance. If no in-workflow tag ancestry check is used, tag rulesets are the enforcement point for release provenance.

### 3) Protected Environments

- Create protected `release` environment with required reviewers.
- Restrict `release` environment deployment refs to intended release refs (version tags).
- Create protected `deployment-tests` environment for manual cloud tests with required reviewers and `main`-only restrictions.

This enforces approval and ref-gating for privileged jobs in [`release.yml`](../.github/workflows/release.yml) and [`tests-deployment.yml`](../.github/workflows/tests-deployment.yml).

### 4) Ownership and Review Governance

- `CODEOWNERS` must reference the real maintainer team with write access (`@exasol/r-d-platform-blue`).
- Require code-owner review in `main` ruleset for security-sensitive paths.

This ensures workflow and release-control changes cannot merge without maintainer approval.

### 5) Secrets and Credential Governance

- Prefer OIDC short-lived credentials over long-lived cloud secrets.
- Move high-risk credentials behind protected environments only.
- Audit and reduce repository/org secret inventory on a regular cadence.

This keeps privileged automation aligned with least-privilege and limits blast radius if workflow code changes.

## Control Alignment (Workflows vs Settings)

- `ci.yml` as the only PR workflow is not sufficient unless PR write-token and secret sharing settings remain disabled.
- Job-level minimal `permissions` are strongest when repository default token permission is restricted.
- Third-party SHA pinning in workflow files is strongest when Actions policy also enforces SHA pinning and allowlists.
- Release environment usage in workflow YAML is effective only when reviewers and ref restrictions are configured in environment settings.
- Tag-triggered release behavior in workflow YAML is effective only when `v*` tag rulesets restrict who can create or modify release tags.

## Ownership and Review Policy

Security-sensitive paths are owned by `@exasol/r-d-platform-blue` via [`CODEOWNERS`](../.github/CODEOWNERS).

Any changes to workflows, shared actions, release controls, or this policy document require code-owner review.
