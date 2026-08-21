## 1. Workflow Planning

- [x] 1.1 Refactor the deployment-test planner to emit filtered cloud and local matrices with row-presence outputs, and verify all supported suite/OS combinations produce the expected matrix sizes
- [x] 1.2 Reject exclusive suite selections with no compatible rows while allowing combined-suite selections to omit an unmatched half, and verify the planner error and skip cases

## 2. Local Deployment Matrix

- [x] 2.1 Convert the local deployment job to Linux AMD64 and macOS ARM64 matrix rows with platform-compatible runner metadata, and verify the expanded workflow structure
- [x] 2.2 Route managed Python to Linux and system Python to macOS while preserving the existing macOS status context and adding a Linux context, and verify each row's resolved action inputs

## 3. Documentation

- [x] 3.1 Update Task and pytest marker descriptions for portable Linux/macOS local deployment tests and verify the task list and pytest collection remain valid
- [x] 3.2 Update CI and testing guidance to describe manual local suite selection, enabled platforms, and deferred platforms, and verify documentation matches workflow inputs

## 4. Verification

- [x] 4.1 Run strict OpenSpec validation, workflow syntax/planner checks, and local pytest collection
- [x] 4.2 Run the repository formatting, linting, unit/integration test, and build checks required by the development guide
- [x] 4.3 Document the post-push Linux AMD64 and macOS ARM64 dispatch commands and acceptance checks for deployment, test execution, cleanup, and final status reporting

## 5. Test Portability Follow-up

- [x] 5.1 Run the full local deployment lifecycle on Linux and macOS with runtime-specific shell capability assertions while retaining macOS guards for VM-only behavior
- [x] 5.2 Remove the local marker from the COS-only diagnostic and delete the obsolete unsupported-platform escape-hatch test and helpers
- [x] 5.3 Verify local test collection, the real Linux lifecycle, the local-install integration subset, strict OpenSpec validation, and required repository checks

## 6. Non-VM Startup Stabilization

- [x] 6.1 Classify wrapped non-VM connection-reset errors as refused and add focused unit coverage for the observed dial-error shape
- [x] 6.2 Run 30 consecutive Linux local lifecycle iterations with temporary diagnostics and remove the diagnostics afterward
- [x] 6.3 Rebuild without diagnostics and run focused runtime, formatting, and strict OpenSpec checks
