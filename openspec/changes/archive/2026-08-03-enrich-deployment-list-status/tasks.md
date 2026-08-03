## 1. Implementation: `cmd/exasol/deployments.go`

- [x] 1.1 Remove `Active` from `deploymentListEntry` and drop the `json:"active"` tag.
- [x] 1.2 Rewrite `deploymentListEntryFor` to drop the `activePath` parameter and the `isRecognizedDeploymentDir` call; call `deploy.GetStatus(ctx, config.NewDeploymentDir(path), false)` and use its `Status` value as `entry.Status` (fall back to `deploymentStatusNotInitialized` on any error, matching the function's existing per-entry error tolerance).
- [x] 1.3 Broaden the preset-identity condition in `deploymentListEntryFor` from `entry.Status == deploymentStatusInitialized` to `entry.Status != deploymentStatusNotInitialized`.
- [x] 1.4 Thread a `context.Context` from `deploymentsListCmd`'s `RunE` (via `cmd.Context()`) through `listDeploymentDirectories` and `deploymentListEntryFor` for the new `deploy.GetStatus` call.
- [x] 1.5 Remove `activeDeploymentDirPath` and its call site in `listDeploymentDirectories`.
- [x] 1.6 Update `deploymentListEntryText` and `renderDeploymentsListText` to drop the `*`/` ` active marker column from the human-readable output.
- [x] 1.7 Update the doc comments on `deploymentListEntry`, `deploymentListEntryFor`, and `listDeploymentDirectories` that describe today's initialized/not_initialized-only behavior and the active-marker computation.

## 2. Go unit tests: `cmd/exasol/deployments_test.go`

- [x] 2.1 Update `TestListDeploymentDirectories_ReportsInitializedForLegacyMarkerOnlyDirectory` to expect `deploymentStatusNotInitialized` (a legacy-marker-only directory with no modern state file is no longer recognized as initialized), or remove it if a rename better reflects the new expectation.
- [x] 2.2 Remove `TestListDeploymentDirectories_MarksActiveEntryFromCwd` and `TestListDeploymentDirectories_NoEntryActiveWhenActiveDirOutsideListedTree` (the `Active` field no longer exists).
- [x] 2.3 Add unit test(s) covering at least one non-`initialized` lifecycle status (e.g. a directory whose state file encodes `WorkflowStateRunning` or `WorkflowStateStopped`) reporting that status and still showing preset identity.
- [x] 2.4 Add a unit test covering a directory whose state file exists but is unparseable, asserting it reports `not_initialized` and does not fail the listing.

## 3. Integration tests: `tests/tests/integration/test_deployments_list.py`

- [x] 3.1 Remove `test_deployments_list_marks_active_entry_from_current_directory`.
- [x] 3.2 Extend `test_deployments_list_reports_named_deployments` (or add a new test) to deploy a named deployment (e.g. via `exasol deploy` on the existing `staging` fixture, if the test environment supports a real/fake deploy) and assert `status == "running"` (or the appropriate reachable state) with `infrastructure`/`installation` still present.
- [x] 3.3 Confirm no remaining test asserts on an `active` key in the JSON output.

## 4. Documentation

- [x] 4.1 Edit the existing `## Unreleased` entry in `CHANGELOG.md` for `exasol deployments list` in place, replacing "their status, and which deployment is currently active" with wording describing the real lifecycle status (no `active` field).

## 5. Validation

- [x] 5.1 Run `task fmt` and `task lint`.
- [x] 5.2 Run the full test suite (`task all` or repo-equivalent) and confirm all `deployments list` unit and integration tests pass.
- [x] 5.3 Manually verify `exasol deployments list --json` and the text output against a deployed-and-running local deployment to confirm `status` reflects reality and no `active` key/marker remains.
