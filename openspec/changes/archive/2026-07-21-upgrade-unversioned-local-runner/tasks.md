## 1. Runner Reconciliation

- [x] 1.1 Add side-effect-free semantic-version probing for installed and embedded runner executables
- [x] 1.2 Reconcile runners during prepare using legacy, compatible upgrade, same-version repair, downgrade, and major-mismatch policies
- [x] 1.3 Replace eligible runners atomically without changing status, stop, or destroy staging behavior

## 2. Behavioral Coverage

- [x] 2.1 Add unit coverage for missing, unversioned, compatible newer, older, equal-content, same-version-different-content, major-mismatch, and invalid embedded runners
- [x] 2.2 Update existing fake-runner coverage to satisfy the version contract without weakening lifecycle assertions
- [x] 2.3 Add a warned internal development override that forces reconciliation with an unversioned embedded runner
- [x] 2.4 Accept the project's `v`-prefixed runner versions and cover release-candidate upgrades

## 3. Verification

- [x] 3.1 Run formatting and focused Go unit tests for the local runtime and deployment workflow
- [x] 3.2 Run repository unit tests and build the launcher with a versioned runner asset
