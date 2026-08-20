## 1. Workflow Planning

- [ ] 1.1 Refactor the deployment-test planner to emit filtered cloud and local matrices with row-presence outputs, and verify all supported suite/OS combinations produce the expected matrix sizes
- [ ] 1.2 Reject exclusive suite selections with no compatible rows while allowing combined-suite selections to omit an unmatched half, and verify the planner error and skip cases

## 2. Local Deployment Matrix

- [ ] 2.1 Convert the local deployment job to Linux AMD64 and macOS ARM64 matrix rows with platform-compatible runner metadata, and verify the expanded workflow structure
- [ ] 2.2 Route managed Python to Linux and system Python to macOS while preserving the existing macOS status context and adding a Linux context, and verify each row's resolved action inputs

## 3. Documentation

- [ ] 3.1 Update Task and pytest marker descriptions for portable Linux/macOS local deployment tests and verify the task list and pytest collection remain valid
- [ ] 3.2 Update CI and testing guidance to describe manual local suite selection, enabled platforms, and deferred platforms, and verify documentation matches workflow inputs

## 4. Verification

- [ ] 4.1 Run strict OpenSpec validation, workflow syntax/planner checks, and local pytest collection
- [ ] 4.2 Run the repository formatting, linting, unit/integration test, and build checks required by the development guide
- [ ] 4.3 Dispatch the Linux AMD64 and macOS ARM64 local workflow rows from a pushed branch and verify deployment, test execution, cleanup, and final status reporting
