# Public Repo CI Security Best Practices

This document captures security best practices for GitHub Actions in public repositories. It focuses on protecting CI/CD from untrusted contributors and supply-chain compromise.

## Threat Model

- Anyone can read and fork a public repository, open pull requests, and propose workflow changes.
- Attackers may try to execute expensive workflows, steal secrets, tamper with release artifacts, or introduce compromised dependencies.
- Security controls should reduce trigger surface, enforce least privilege, and harden dependency trust.

## Controls and Rationale

### 1) Trigger Workflows from Trusted Events

- Use `pull_request` for untrusted CI (lint/build/unit tests only).
- Use `push` and `workflow_dispatch` for privileged workflows (deploy/release/signing/cloud access).
- Avoid `pull_request_target` for builds/tests of contributor code.

Rationale: Trigger choice defines who can execute code in your CI context. In public repos, pull request events are the primary untrusted input path.

### 2) Use a Two-Lane Trust Model for Public Repositories

- Lane A (`pull_request`): run only low-risk checks, with no secrets and no privileged permissions.
- Lane B (`push`/`workflow_dispatch`): run privileged workflows only for trusted contributors and protected refs.
- Keep these lanes separate so untrusted code never reaches privileged jobs.

Rationale: Public contribution and strong security are compatible when untrusted and trusted execution contexts are separated.

### 3) Minimize `GITHUB_TOKEN` Permissions

- Set repository default token permissions to restricted (read-only baseline).
- Declare `permissions` explicitly in each workflow/job.
- Keep "Allow GitHub Actions to create and approve pull requests" disabled unless required.

Rationale: Most workflow compromises become high-impact only when the token has write scope.

### 4) Use Secrets and Cloud Access Boundaries

- Use OIDC with short-lived cloud credentials instead of long-lived secrets.
- Scope cloud trust policies to repository, ref, and environment.
- Put high-risk secrets behind protected environments and required reviewers.

Rationale: Even if workflow code is modified, narrow trust and approval gates limit blast radius.

### 5) Harden Third-Party Dependencies

- Pin third-party actions to full commit SHAs.
- Enable repository policy requiring full-length SHA pinning.
- Allow only approved actions/reusable workflows.
- Use Dependabot to keep Actions dependencies updated.
- Verify checksums/signatures for downloaded tools and binaries.

Rationale: Tags can move, external repositories can be compromised, and mutable downloads are a common supply-chain entry point.

### 6) Protect Branch and Tag Mutation

- Use rulesets/branch protection on the default branch.
- Require pull requests, required checks, and review gates before merge.
- Block force pushes and deletions on protected refs.
- Protect release tag patterns (for example `v*`) with strict creation/update/delete restrictions.

Rationale: Ref protection prevents bypassing CI and protects release provenance.

### 7) Protect Workflow Changes

- Add `.github/workflows/**` and `.github/actions/**` to `CODEOWNERS`.
- Require code owner approval for workflow changes.

Rationale: Workflow files define CI trust boundaries and must be reviewed like privileged infrastructure code.

### 8) Gate High-Risk Release Operations

- Keep release jobs separate from regular CI jobs.
- Require protected environment approval for publishing/signing.
- Validate release refs before publishing (for example, verify tag commit belongs to protected branch history).

Rationale: Strong release gates prevent accidental or malicious publication from untrusted refs.

### 9) Prefer Ephemeral Hosted Runners for Public Repos

- Prefer GitHub-hosted ephemeral runners for public workflows.
- Avoid exposing self-hosted runners to untrusted events.

Rationale: Long-lived runner state increases persistence and lateral-movement risk.

### 10) Operational Security Hygiene

- Regularly review Actions settings, rulesets, and bypass lists.
- Reduce artifact/log retention where practical.
- Rotate credentials immediately after any suspected exposure.

Rationale: Security posture drifts over time without periodic governance checks.

## Authoritative References

- Managing GitHub Actions settings for a repository: https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/enabling-features-for-your-repository/managing-github-actions-settings-for-a-repository
- Secure use reference: https://docs.github.com/en/actions/reference/security/secure-use
- Workflow syntax (`push` branch/tag filters): https://docs.github.com/en/actions/reference/workflow-syntax-for-github-actions
- Deployment branches/tags for environments: https://docs.github.com/en/actions/reference/workflows-and-actions/deployments-and-environments
- Rulesets and available rules: https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-rulesets/available-rules-for-rulesets
- About protected branches: https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-protected-branches/about-protected-branches
- Manually running workflows (`workflow_dispatch`): https://docs.github.com/en/actions/managing-workflow-runs/manually-running-a-workflow
- Artifact attestations: https://docs.github.com/en/actions/how-tos/secure-your-work/use-artifact-attestations
