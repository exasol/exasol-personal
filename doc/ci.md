# CI/CD

The project uses GitHub Actions for continuous integration and delivery. Workflow definitions are in `.github/workflows/`.

Security policy for public-repo CI is defined in [Repository Security and Automation Governance](repository_security_spec.md) and [CI Security Best Practices](ci_security_best_practices.md).

## Automated Workflows

### CI Pipeline (`ci.yml`)

Runs automatically on every push to `main` and on pull requests targeting `main`:

- **Go Linting** - Runs `golangci-lint` and `tflint`
- **Python Linting** - Runs `ruff` and `mypy` on test code
- **Unit Tests** - Runs Go unit tests with coverage
- **Integration Tests** - Runs Python integration tests on Linux and Windows

Build and integration tests run on both `ubuntu-latest` and `windows-latest`, so Windows-only behavior — including Windows local deployments — is covered on every pull request. Unit tests and linting run on Linux only.

This is the only workflow that runs contributor code in pull request context. It is intentionally non-privileged and does not use deployment/release credentials.
All CI jobs declare explicit minimal permissions.

### Dependabot Auto-Merge (`dependabot-auto-merge.yml`)

Runs for Dependabot pull requests and enables auto-merge for Go and Python patch and minor version updates after required branch protection checks pass.
Eligibility is derived from the pull request's branch name: Go and Python updates that land on their ecosystem's grouped update branch (see `groups` in [`dependabot.yml`](../.github/dependabot.yml)) are routine and are approved immediately, since Dependabot cooldown already delayed their creation.
Any Go or Python Dependabot branch that falls outside its group is treated as a security update — Dependabot never assigns security updates to a group — and waits 7 days after PR creation before the workflow approves it and enables auto-merge, since Dependabot cooldown does not delay security updates.
GitHub Actions updates remain manual review items regardless of grouping.
Repository auto-merge must be enabled for this workflow to queue eligible pull requests.
The workflow uses `pull_request_target` only for pull request metadata (branch name, creation time, review status) and GitHub pull request operations; it does not check out or execute pull request code, and does not depend on Dependabot's commit-message metadata.
Scheduled rechecks run every 6 hours to pick up eligible open Dependabot PRs after the merge delay has elapsed.

Dependabot version updates use built-in cooldown periods before PR creation:

- GitHub Actions updates wait 7 days and remain manual.
- Go and Python patch updates wait 2 days.
- Go and Python minor updates wait 7 days.

The merge delay is separate from Dependabot cooldown: cooldown delays routine PR creation, while the workflow delay defers approval and auto-merge for ungrouped (security) PRs after they exist.
Auto-merge does not itself rebase Dependabot PRs; branch protection and Dependabot's own update behavior determine whether stale branches are refreshed before merging.

### Release Pipeline (`release.yml`)

Triggered automatically when a version tag is pushed (e.g., `v1.2.3`):

- Builds binaries for all platforms (Linux, macOS, Windows)
- Runs tests
- Creates GitHub release with artifacts
- Uses a protected `release` environment for release/signing approval gates
- See [Release Process](release.md) for details

### Merge Workflow (`merge.yml`)

Runs automatically on every push to `main`:

- Builds binaries for Windows and macOS platforms
- Uploads build artifacts for verification

This ensures multi-platform compatibility is validated on the main branch.

### Orphaned Deployment Cleanup (`cleanup-orphaned-deployments.yml`)

Runs nightly (and on manual dispatch) to delete cloud resources left behind by failed or interrupted deployment tests, using the [cleanup tool](../tools/cleanup/README.md).
Deployments older than the configured age threshold are removed across AWS, Azure, and Exoscale.

Deployments in the region CI currently deploys to (`AWS_REGION`) are removed once they are a day old, as are Azure and Exoscale deployments.
AWS resources outlive a change of deployment region, so the regions CI deployed to previously are swept as well, but on a longer grace period of three days, since nothing left there belongs to a recent test run.
Each region runs as its own job with its own threshold, so add the outgoing region as a row whenever `AWS_REGION` changes, and drop a row once its region is known to be empty.
Splitting by region also keeps a deployment from matching two search targets at once, which the cleanup tool rejects: AWS reports global resources such as IAM roles in `us-east-1` regardless of which region holds the deployment's instances.

## Manual Workflows

### Documentation Publication (`docs.yml`)

Maintainers publish versioned user documentation with the manually dispatched documentation
workflow. GitHub Pages must be enabled with **GitHub Actions** as its source, and the existing
`github-pages` environment must allow deployments only from `main`.

The workflow accepts two inputs:

- `operation`: `publish` or `delete`.
- `target`: an existing Git tag for publication, such as `v2.3.0-rc1`, or a published version for
  deletion, such as `2.3.0-rc1`.

Publication builds the documentation and validates its internal links from the selected tag before
updating the version catalog. Stable versions update `latest` and the site root; release candidates
remain independently selectable. Deletion preserves all other versions and rejects removal of the
version referenced by `latest` until a newer stable version is published.

All operations are serialized in request order. If Pages deployment fails after the version
catalog is updated, rerun the same operation to deploy the complete stored catalog. A repeated
deletion succeeds when the selected version is already absent and redeploys that catalog.

### Deployment Tests (`tests-deployment.yml`)

Full end-to-end tests that create real cloud or local deployments. These are expensive and slow, so they run only when needed:

**Trigger manually via:**
- GitHub Actions UI: [tests-deployment.yml](https://github.com/exasol/exasol-personal/actions/workflows/tests-deployment.yml) → "Run workflow"

Security guards:
- Runs only by manual trigger (`workflow_dispatch`)
- Uses OIDC and short-lived AWS credentials
- Should be protected by an environment approval gate and ref restrictions in repository settings

Workflow input:
- `suite`: test-suite selector (`all`, `cloud`, `local`; default `all`)
- `os`: OS selector for both cloud and local matrices (`all`, `ubuntu-latest`, `windows-latest`, `macos-latest`; default `all`)
- The workflow filters declarative cloud and local test plans before matrix expansion, so non-selected jobs are not created.
- Current enabled cloud rows:
  - AWS runs `tests-deployment` (installation + infrastructure lanes)
  - Azure runs `tests-deployment-infrastructure`
  - Exoscale runs `tests-deployment-infrastructure`
- Current enabled local rows:
  - Linux AMD64 runs `tests-deployment-local` on `ubuntu-latest`
  - Windows AMD64 runs `tests-deployment-local` on `windows-latest`
  - macOS ARM64 runs `tests-deployment-local` on the self-hosted virtualization runner
  - Linux ARM64 coverage is deferred.
  - The Windows row runs most of the local suite. Two pseudo-terminal cases remain POSIX-only, and the VM sizing cases remain macOS-only, so Windows covers 22 of the 27 selected tests.
- After pushing a branch, dispatch one local row with `task github:trigger-deployment-tests SUITE=local OS=ubuntu-latest` (or `OS=windows-latest`, `OS=macos-latest`), then verify deployment, tests, cleanup, and the final commit status.
- Credential bootstrap:
  - AWS via OIDC role assumption
  - Azure via OIDC (`azure/login`)
  - Exoscale via `EXOSCALE_API_KEY` / `EXOSCALE_API_SECRET` secrets
  - Azure identifiers are sourced from GitHub secrets: `AZURE_CLIENT_ID`, `AZURE_TENANT_ID`, `AZURE_SUBSCRIPTION_ID`

**Warning:** Cloud rows create real infrastructure and incur costs; local rows create a real database deployment on the selected runner.

### Integration Tests (`tests-integration.yml`)

The same integration suite the CI pipeline runs, dispatchable on its own when a targeted run is wanted without pushing a new commit or opening a pull request.

**Trigger manually via:**
- GitHub Actions UI: [tests-integration.yml](https://github.com/exasol/exasol-personal/actions/workflows/tests-integration.yml) → "Run workflow"
- After pushing a branch: `task github:trigger-integration-tests OS=windows-latest`

Workflow input:
- `os`: OS selector (`all`, `ubuntu-latest`, `windows-latest`; default `all`)

On Windows, a failing run also uploads Podman machine state, container inspection and logs, listening ports, and the test deployment directory as a diagnostics artifact.

## AWS Identity Provider and IAM Role for Deployment Tests

Deployment tests authenticate to the "exa-aws-dev-platform" AWS account using GitHub Actions’ OpenID Connect (OIDC). This avoids long‑lived AWS secrets and issues short‑lived credentials per workflow run.

What’s set up in AWS:
- An IAM OIDC identity provider `token.actions.githubusercontent.com` for GitHub with provider URL `https://token.actions.githubusercontent.com` and audience `sts.amazonaws.com`.
- An IAM role `PlatformGithubWorkflows` trusted by that OIDC provider. The role’s trust policy limits which repositories/branches/environments can assume it using conditions on `token.actions.githubusercontent.com:sub` and `token.actions.githubusercontent.com:aud`.

Where it’s used in CI:
- The workflow `tests-deployment.yml` configures AWS via a shared action that consumes two repository variables:
	- `AWS_CI_ROLE_PLATFORM` — ARN of the IAM role to assume in the platform account
	- `AWS_REGION` — target region for deployments
- The job permissions include `id-token: write` to allow OIDC token issuance.

Maintenance tips:
- Prefer least privilege: attach only the permissions required for deployment tests to the IAM role.
- Scope trust policies narrowly to this repository/branch/environment using the `sub` claim; adjust as the workflow structure evolves.
- When rotating roles or changing account setup, update the `AWS_CI_ROLE_PLATFORM` variable with the new role ARN; for region changes, update `AWS_REGION` and add the outgoing region to the cleanup workflow's matrix.
- Audit and monitor with AWS CloudTrail; review trust and permission policies regularly.

Authoritative references:
- GitHub Docs — Configuring OIDC in AWS: https://docs.github.com/en/actions/deployment/security-hardening-your-deployments/configuring-openid-connect-in-amazon-web-services
- AWS IAM — Creating OIDC identity providers: https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_providers_create_oidc.html
- AWS IAM — Configuring a role for GitHub OIDC: https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_create_for-idp_oidc.html#idp_oidc_Create_GitHub
- Action — aws-actions/configure-aws-credentials: https://github.com/aws-actions/configure-aws-credentials

## Governance Controls

Changes to workflow definitions, shared GitHub Actions, and CI security documents are protected by [CODEOWNERS](../.github/CODEOWNERS).
GitHub Actions dependencies are updated automatically via [Dependabot](../.github/dependabot.yml).
