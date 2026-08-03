## Why

SPOT-31887 reports that `exasol deployments list -j` always shows `status: "initialized"` and `active: false` regardless of whether a deployment has actually been deployed, is running, or has stopped. Investigation confirmed the implementation matches the spec exactly — both fields were deliberately scoped narrowly when the command was introduced: `status` only distinguishes "has an init marker" from not, to avoid the cost of `deploy.Status()`'s per-directory lock and DB probe, and `active` means "would be selected with no flags from the current working directory," not "is running." That scoping doesn't meet a reasonable user's expectation of what a deployment listing should show, and a cheaper primitive already exists in this codebase (`deploy.GetStatus(ctx, deployment, false)`, used lock-free and probe-free elsewhere) that closes the gap without reintroducing the original performance concern.

## What Changes

- `exasol deployments list` reports each deployment directory's real lifecycle status (`not_initialized`, `initialized`, `operation_in_progress`, `interrupted`, `deployment_failed`, `stopped`, `running` — the same vocabulary `exasol status` uses) instead of only `initialized`/`not_initialized`, computed via the existing lock-free, no-DB-probe `deploy.GetStatus(ctx, deployment, false)` read per directory.
- Preset identity is now shown for any listed directory whose status is not `not_initialized` (previously gated on the literal value `initialized`), so `running`/`stopped`/etc. entries keep showing their infrastructure and installation preset.
- **BREAKING**: The `active` field (and its text-output `*` marker) is removed entirely. It indicated CWD-based directory-selection precedence, which was judged unintuitive and not useful in a command that lists every deployment directory at once. `exasol status` and other per-deployment commands are unaffected — this only removes the field from `deployments list` output.
- A deployment directory whose state file is present but unreadable/corrupt is still reported as `not_initialized`, matching today's existing tolerant behavior (one bad directory degrades gracefully rather than failing the whole listing).
- The dedicated legacy-marker recognition guarantee is folded into the general status determination rather than kept as a separate spec scenario; no behavior is promised beyond what `deploy.GetStatus` itself determines.

## Capabilities

### Modified Capabilities
- `deployment-directory-listing`: replaces the "report initialization status" requirement with a "report deployment status" requirement covering the full lifecycle vocabulary; removes the "indicate the currently active deployment directory" requirement and its scenarios; updates the JSON-output scenario to drop the active field; broadens the preset-identity condition from `status == initialized` to `status != not_initialized`.

## Impact

- `cmd/exasol/deployments.go`: `deploymentListEntry` struct (drop `Active`), `deploymentListEntryFor` (call `deploy.GetStatus` instead of the binary `isRecognizedDeploymentDir` check; broaden the preset-identity condition), `listDeploymentDirectories` (drop `activeDeploymentDirPath` call), `activeDeploymentDirPath` (removed), `renderDeploymentsListText`/`deploymentListEntryText` (drop the active marker column).
- `CHANGELOG.md`: edit the existing `Unreleased` entry for `exasol deployments list` in place (it hasn't shipped yet) rather than adding a new entry.
- `tests/tests/integration/test_deployments_list.py`: remove the active-marker test, update status assertions to the new lifecycle values, extend preset-identity assertions to non-`initialized` statuses.
- No change to `exasol status`, `deploy.GetStatus`, or the CWD-selection resolver (`resolveDeploymentDirFromValues`) — reused as-is, not modified.
