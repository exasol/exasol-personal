## Context

The launcher embeds one Exasol Local runner and copies it into each deployment's launcher-owned runtime directory. The current staging operation stops after finding any existing file, so deployments keep the runner from their first installation indefinitely. The runner shipped with Exasol Personal 2.0 predates runner version reporting; the migration runner and all future runners provide a side-effect-free `version` command whose trimmed stdout is a semantic version.

Runner reconciliation happens through `internal/localruntime`, while the deployment workflow guarantees that `Prepare` runs after a deployment is permitted to start and before the runner's `init` or `start` command. Status and stop operations may execute while a daemon from the installed runner is active and therefore must not replace it.

## Goals / Non-Goals

**Goals:**

- Upgrade the one unversioned production runner to the embedded migration runner before start.
- Automatically apply newer compatible runner patch and minor versions.
- Avoid automatic runner downgrades and major-version changes.
- Repair a same-version runner whose bytes differ from the trusted embedded binary.
- Make replacement atomic and safe to retry after interruption.

**Non-Goals:**

- Add a user-facing command for opting into major runner migrations.
- Update a runner while its VM daemon may be running.
- Preserve unsupported user modifications inside the launcher-owned runner path.
- Change cloud deployment lifecycle behavior.

## Decisions

### Probe the installed and embedded runners through the runner contract

The launcher will materialize the embedded runner as a temporary executable in the runtime directory and invoke `version` on both candidates. Version output uses the project's `v`-prefixed semantic-version convention and is parsed tolerantly after trimming whitespace, including release-candidate suffixes. This keeps the runner binary authoritative and avoids duplicating its version in launcher source or parsing release URLs.

Failure to obtain a valid version from the installed runner classifies it as the unversioned legacy runner and permits the one-time replacement. Failure to obtain a valid version from the embedded runner is a packaging error and blocks start.

Alternative considered: identify the legacy runner by a checked-in digest. That distinguishes custom binaries, but adds historical release metadata solely for a one-time migration and is unnecessary because the runtime path is launcher-owned.

### Reconcile only during prepare

`Prepare` will reconcile the runner before VM initialization or start. Status, stop, and destroy retain the installed runner so a replacement cannot introduce a new CLI/daemon contract while the old daemon is active.

Alternative considered: reconcile from the shared executable-presence check. That check is also used by status and stop, making its lifecycle timing unsafe for upgrades.

### Apply an upgrade-only semantic-version policy

An unversioned installed runner is replaced by the versioned embedded migration runner. For versioned runners, a newer embedded patch or minor within the same major is installed; an older embedded runner is never installed. Equal semantic versions with different bytes are repaired from the trusted embedded copy. A major mismatch preserves the installed runner and emits an actionable warning.

Alternative considered: overwrite on every byte difference. That can silently downgrade runners or cross an incompatible major boundary.

### Replace atomically from a same-directory temporary file

The launcher writes the embedded bytes to an executable temporary file in the runner directory, validates its version, and renames it over the installed path when policy permits. A same-directory rename prevents a partially written runner from becoming the managed executable. Cleanup removes unused candidates.

Alternative considered: truncate and rewrite the installed runner. Interruption could leave a deployment without an executable runner.

### Provide a temporary internal forced-reconciliation bypass

Setting `EXASOL_LOCAL_FORCE_RUNNER_RECONCILIATION=1` allows an embedded runner without a valid version to participate in reconciliation. When its bytes differ from the installed runner, the launcher atomically installs it without semantic-version compatibility checks and emits a warning; equal bytes remain a no-op. This enables migration testing before a versioned runner is available without weakening the default production path.

Alternative considered: skip reconciliation when the embedded runner is unversioned. That would preserve the exact runner the migration build is intended to replace.

## Risks / Trade-offs

- A custom unversioned runner in the launcher-owned path is replaced → Treat that path as managed state and require development/test runners to implement `version` or be embedded as the candidate.
- A version command fails for a reason other than being unsupported → The legacy rule repairs the installed runner from the trusted embedded copy; candidate failures still block start.
- A future launcher retains the unversioned exception across a major boundary → Keep the exception tied to the migration-capable runner line and require explicit major-upgrade design before changing the embedded runner major.
- Warning while retaining an older major assumes launcher compatibility → Preserve the narrow existing runner command contract and add an explicit major migration workflow before making incompatible calls.
- The internal bypass can cross compatibility boundaries → Keep it undocumented for end users, require an exact opt-in value, and warn whenever it installs an unversioned runner.

## Migration Plan

1. Release a versioned migration runner through the existing embedded-runner packaging path.
2. On the first subsequent install or start, replace the unversioned runner atomically before invoking it.
3. On later starts, use semantic versions and content equality to select no-op, compatible upgrade, repair, or warning behavior.
4. Roll back the launcher without downgrading the installed runner; an older bundled runner is retained rather than installed.

## Open Questions

None for the 2.0 migration. A future major runner release requires a separate explicit opt-in migration design.
