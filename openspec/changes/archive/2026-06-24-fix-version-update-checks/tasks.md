## 1. Version Comparison

- [x] 1.1 Add a semantic version comparison helper for launcher update checks.
- [x] 1.2 Make automatic update checks report updates only when the reported latest version is semantically newer.
- [x] 1.3 Ensure invalid or missing version data does not produce automatic update hints.

## 2. User-Facing Output

- [x] 2.1 Update `exasol version --latest` text output for newer, equal, and older reported versions.
- [x] 2.2 Preserve existing JSON output shape for `exasol version --latest --json`.

## 3. Documentation

- [x] 3.1 Document the launcher version-check semantic ordering policy.
- [x] 3.2 Document prerelease handling for release candidates and final releases.

## 4. Tests and Validation

- [x] 4.1 Add Go unit tests for `2.0.0-rc1` versus `1.4.1`, equal versions, newer patch versions, and prerelease-versus-final cases.
- [x] 4.2 Add Go unit tests for user-facing latest-version text.
- [x] 4.3 Run focused unit tests for the changed packages.
- [x] 4.4 Run repository-required formatting and validation commands.

## 5. Output Stream Policy

- [x] 5.1 Document that explicit version command output goes to stdout.
- [x] 5.2 Document that implicit update hints are metadata and go to stderr.
- [x] 5.3 Route explicit version command output through the terminal output queue.
- [x] 5.4 Render human-readable latest-version output with an embedded template.
- [x] 5.5 Make `exasol version --json` emit structured JSON.
