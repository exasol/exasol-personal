# CI/CD

The project uses GitHub Actions for continuous integration and delivery. Workflow definitions are in `.github/workflows/`.

Security policy for public-repo CI is defined in [Repository Security and Automation Governance](repository_security_spec.md) and [CI Security Best Practices](ci_security_best_practices.md).

## Automated Workflows

### CI Pipeline (`ci.yml`)

Runs automatically on every push to `main` and on pull requests targeting `main`:

- **Go Linting** - Runs `golangci-lint` and `tflint`
- **Python Linting** - Runs `ruff` and `mypy` on test code
- **Unit Tests** - Runs Go unit tests with coverage
- **Integration Tests** - Runs Python integration tests

This is the only workflow that runs in pull request context. It is intentionally non-privileged and does not use deployment/release credentials.
All CI jobs declare explicit minimal permissions.

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

## Manual Workflows

### Deployment Tests (`tests-deployment.yml`)

Full end-to-end tests that provision real AWS infrastructure. These are expensive and slow, so they run only when needed:

**Trigger manually via:**
- GitHub Actions UI: [tests-deployment.yml](https://github.com/exasol/exasol-personal/actions/workflows/tests-deployment.yml) → "Run workflow"

Security guards:
- Runs only by manual trigger (`workflow_dispatch`)
- Uses OIDC and short-lived AWS credentials
- Should be protected by an environment approval gate and ref restrictions in repository settings

**Warning:** These tests create real AWS resources and incur costs.

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
- When rotating roles or changing account setup, update the `AWS_CI_ROLE_PLATFORM` variable with the new role ARN; for region changes, update `AWS_REGION`.
- Audit and monitor with AWS CloudTrail; review trust and permission policies regularly.

Authoritative references:
- GitHub Docs — Configuring OIDC in AWS: https://docs.github.com/en/actions/deployment/security-hardening-your-deployments/configuring-openid-connect-in-amazon-web-services
- AWS IAM — Creating OIDC identity providers: https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_providers_create_oidc.html
- AWS IAM — Configuring a role for GitHub OIDC: https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_create_for-idp_oidc.html#idp_oidc_Create_GitHub
- Action — aws-actions/configure-aws-credentials: https://github.com/aws-actions/configure-aws-credentials

## Governance Controls

Changes to workflow definitions, shared GitHub Actions, and CI security documents are protected by [CODEOWNERS](../.github/CODEOWNERS).
GitHub Actions dependencies are updated automatically via [Dependabot](../.github/dependabot.yml).
