## Why

Local shell commands can replace the actual reason a shell could not be opened with unrelated database-reachability guidance. The existing shell and reachability requirements also describe runtime, transport, and endpoint architecture instead of the durable CLI behavior users need: accurate failures and actionable next steps.

## What Changes

- Report the actual reason a local shell command failed instead of replacing it with unrelated connectivity guidance, including explicit platform-specific unsupported errors.
- Define supported and unsupported shell behavior at the user-facing CLI boundary.
- Report recognized local connectivity problems with platform-appropriate corrective guidance while retaining the underlying command failure.
- Describe local diagnostics in terms of deployment usability, exposed-service reachability, and database readiness rather than runtime or transport architecture.
- Move shell requirements touched by this change to the user-facing local deployment capability and keep implementation contracts in the design and tests.
- Strengthen runtime shell tests to verify exact argument boundaries, working directory, standard streams, and command-failure propagation.
- Add regression coverage for shell-support deployment metadata, conditional connection instructions, and correction of staged artifact permissions.
- Remove obsolete test-fixture behavior that only supported the retired SSH-oriented implementation.
- Document the corrected user-visible shell error behavior in the changelog.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `exasol-local-deployment`: Define supported shell behavior and accurate shell failures at the CLI boundary.
- `local-reachability-diagnostics`: Consolidate reachability failures into an actionable CLI error contract and describe diagnostics through observable deployment health.

## Impact

The change affects local shell error handling, reachability diagnostics, and their supporting implementation tests. User-visible failures become more accurate and actionable; no command-line syntax, deployment JSON schema, or external dependency changes are introduced.
