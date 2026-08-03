## Context

`exasol deployments list` (added in `openspec/changes/archive/2026-07-14-add-named-deployment-directories`) currently determines each entry's `status` via `isRecognizedDeploymentDir` (`cmd/exasol/deployment_dir_resolution.go`) — a marker-file existence check (modern state file, deployment version marker, or legacy `.workflowState.json`) — collapsed to just `initialized`/`not_initialized`. That change's design doc justified this shallowness by contrasting it with `exasol status`'s `deploy.Status()`, which acquires a per-directory lock and probes the database/local VM; running that across every directory in a listing would be slow and could contend with an operation already running against one of them.

That justification describes the locked, connection-checking path only. `internal/deploy/status.go` also exposes `GetStatus(ctx, deployment, checkConnection)`, and the codebase already calls it with `checkConnection=false` and no lock in two places — `internal/deploy/info.go` (under an already-held lock) and `internal/deploy/shared.go`'s `newBlockedStateError` (with no lock at all, per its comment: "these guards have no context; a background read is side-effect free"). That call reads only the persisted workflow-state JSON and returns one of seven real lifecycle states (`not_initialized`, `initialized`, `operation_in_progress`, `interrupted`, `deployment_failed`, `stopped`, `running`) at the same cost class as the marker-file check `deployments list` already performs once per directory.

SPOT-31887 reports that `status` and `active` never reflect real deployment state. Investigation (see the change proposal) confirmed both fields work exactly as originally specified — the gap is in what the original spec promised, not a defect in the implementation. This design closes that gap for `status` using the cheap primitive above, and removes `active` outright per product decision (it answered a different, less useful question — CWD-selection precedence — that doesn't belong in a command listing every directory at once).

## Goals / Non-Goals

**Goals:**
- Report each listed deployment directory's real lifecycle status, using the same vocabulary and cost profile (no lock, no DB/VM probe) as the existing `checkConnection=false` `GetStatus` call sites.
- Keep the listing's existing failure-tolerance: one directory's bad state never fails the whole command.
- Remove the `active` field and its CWD-selection computation from `deployments list` entirely.
- Broaden preset-identity display to any status other than `not_initialized`.

**Non-Goals:**
- Changing `exasol status`, `deploy.Status()`, or `deploy.GetStatus` themselves — reused as-is.
- Preserving the dedicated legacy-marker-recognition guarantee as its own spec scenario. If simplifying the recognition path drops that specific guarantee, that's accepted; `deploy.GetStatus`'s own notion of "initialized" (a readable modern state file) becomes the sole source of truth.
- Detecting or reporting *why* a state file is unreadable (a distinct "broken" status). Corrupt/unreadable state is folded into `not_initialized`, matching today's existing tolerance for any per-entry error.
- Live connection/VM probing (`checkConnection=true`) for the listing. `running` means "the persisted workflow state says running," not "the database was just probed and is ready" (that distinction, `database_ready`/`database_connection_failed`, only exists when `checkConnection=true` and is out of scope here).

## Decisions

### Replace the recognition-then-binary-status computation with `deploy.GetStatus(ctx, deployment, false)`

`deploymentListEntryFor` currently calls `isRecognizedDeploymentDir(path)` to decide `initialized` vs. `not_initialized`, then separately calls `deploy.ResolveDeploymentPresetIdentity` when recognized. It will instead call `deploy.GetStatus(ctx, config.NewDeploymentDir(path), false)` directly and use its returned `Status` value as the entry's status. `GetStatus` already treats a missing or unparseable state file as `not_initialized` (via `errors.Is(err, config.ErrMissingConfigFile)` in `GetStatus` itself, and any other read/parse error propagating up — `deploymentListEntryFor` already tolerates per-entry errors by degrading to `not_initialized`, so no new error-handling path is needed).

This means `isRecognizedDeploymentDir` is no longer called from `deployments list`. It keeps its existing callers (current-working-directory recognition in `resolveDeploymentDirFromValues`) — those are unaffected; this change only stops reusing it for the listing's status determination.

Alternatives considered:
- Keep `isRecognizedDeploymentDir` as a pre-check and only call `GetStatus` when recognized (to explicitly preserve today's legacy-marker guarantee, falling back to `initialized` when `GetStatus` would otherwise say `not_initialized`). Rejected per explicit product decision: the legacy-marker case does not need special preservation, and the extra branch would exist solely to protect a guarantee that is no longer required. A single `GetStatus` call is simpler and has one source of truth for "is this initialized."

### Broaden preset-identity display to `status != not_initialized`

`deploymentListEntryFor` resolves `deploy.ResolveDeploymentPresetIdentity` only when the entry's status was the literal string `initialized`. With the full lifecycle vocabulary, that check becomes `entry.Status != deploymentStatusNotInitialized` — a `running`, `stopped`, `interrupted`, `operation_in_progress`, or `deployment_failed` deployment still has a persisted (or derivable) preset identity and should keep showing it, exactly as an `initialized` one does today.

### Remove `active` and its computation outright, no replacement field

`deploymentListEntry.Active`, `activeDeploymentDirPath()`, and the call to it from `listDeploymentDirectories` are deleted. `deploymentListEntryFor` no longer takes an `activePath` parameter. The text renderer (`deploymentListEntryText`) drops the `*`/` ` marker column — each line becomes `<name> status=<status>[ preset=<infra>/<install>] path=<path>`, one field shorter than today.

`resolveDeploymentDirFromValues` and the CWD-selection precedence it implements are untouched — they remain in use by every other command's directory resolution. Only `deployments list`'s use of it (solely to compute the now-removed `active` field) goes away.

Alternatives considered:
- Rename `active` to something less ambiguous (e.g. `selected`) instead of removing it. Rejected per explicit product decision: the CWD-selection concept was judged not useful in a command whose purpose is to enumerate every deployment directory, not to answer "which one would a bare command hit."

### No new "broken" status value for unreadable state

A state file that exists but fails to parse is reported as `not_initialized`, identical to a missing state file. This matches the pre-existing tolerance already documented on `deploymentListEntryFor` ("never fails on its own... reported as not_initialized rather than aborting the whole listing") and avoids introducing a status value with no equivalent in `exasol status`'s own vocabulary.

## Risks / Trade-offs

- [A deployment directory whose runtime crashed outside the CLI (e.g. a killed container) keeps showing `running` until a command that does check the connection/VM runs] → Accepted; this is the same staleness already accepted by `GetStatus(ctx, deployment, false)`'s two existing call sites (`info`, blocked-state guidance). `exasol status` remains the authority for a live, verified check.
- [Dropping `isRecognizedDeploymentDir` in favor of `GetStatus`'s own recognition means a directory recognized only via the legacy `.workflowState.json` marker or the deployment-version marker file — but with no modern state file — now reports `not_initialized` instead of today's `initialized`] → Accepted per explicit product decision; no migration path is provided for this edge case.
- [Removing `active` is a breaking change to the (unreleased) `deployments list` JSON shape] → Low risk: `exasol deployments list` has not shipped in a stable release (still under `## Unreleased` in `CHANGELOG.md`), so there are no external consumers depending on the current shape yet.

## Migration Plan

Not applicable as a runtime migration — this only changes CLI output shape before the feature's first stable release. `CHANGELOG.md`'s existing `Unreleased` entry for `exasol deployments list` is edited in place to describe the corrected behavior rather than appended to.
