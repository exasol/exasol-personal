## Why

Exasol Personal currently makes users create, choose, and remember a deployment directory before the product has shown value. This adds friction for local-first workflows and can create cloud cost risk when users lose the directory needed to manage or destroy resources.

## What Changes

- Add a transparent default deployment directory at `~/.exasol/personal/deployments/default`.
- Resolve deployment directories using this precedence: explicit `--deployment-dir`, valid current working directory, then the default directory.
- Automatically create the default directory for commands that initialize or install a deployment.
- Keep support for explicit and multiple deployment directories.
- Print a clear message whenever a command resolves to the default directory.
- Include the active deployment directory in `exasol status` output.
- Make `exasol status` report `not_initialized` for uninitialized resolved directories instead of failing only because initialization has not happened yet.
- Keep commands that require an initialized deployment failing clearly when the resolved directory is not initialized, with errors that include the resolved path.
- Update user-facing help and documentation for the default location, precedence, override behavior, and cloud cleanup implications.

## Capabilities

### New Capabilities
- `deployment-directory-resolution`: The CLI resolves, reports, and uses a deployment directory without requiring users to manually choose one for the default workflow.

### Modified Capabilities

## Impact

- Affected CLI command wiring for deployment-directory flags, root pre-run behavior, compatibility enforcement, deployment logging setup, and version update checks.
- Affected status output format for both text and JSON status responses.
- Affected initialization and install workflows when no deployment directory flag is provided.
- Documentation and help text updates in end-user CLI guidance.
- No new external dependencies and no removal of support for multiple deployment directories.
