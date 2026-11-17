## Context

SPOT-34071 follows `align-macos-launcher-paths`. Startup migration currently formats every retained deployment as an error and writes warnings before Cobra parses arguments. PR #520 also adds warnings to named/default selected-directory notices. Terminal helpers already support CTAs, stderr routing and JSON suppression. Migration logic lives in `startup_migration.go`, while legacy sources, fallback, explicit-path redirection and listing are spread across permanent path and command files.

## Goals / Non-Goals

**Goals:** one daily CTA per retained deployment in text mode, safe JSON output, automatic migration independent of reminders, persistent concurrency-safe reminder claims, and a bounded removal surface for this layout migration.

**Non-Goals:** a general migration or notification framework, a removal release, changes to the deployment lock protocol, or migration of intermediate unreleased layouts.

## Decisions

### Separate deferral from failure

Represent a running, locked or unknown-state deployment as a structured deferred result with its paths, reason and applicable recovery action. Moves/deletions and genuine failures remain operational notices. Keep a failure to probe running state visible as a diagnostic as well as explaining the deferral in the eligible CTA. Name collisions and failed renames remain warnings on every attempt; the daily limit applies to guidance about expected deferral, not to correctness reporting.

Render the CTA in the command layer. Include old and target paths, why the deployment remains usable in place, the recovery action and the temporary nature of legacy support. Use a named stop command where unambiguous; use safely quoted explicit paths for collision-sensitive recovery. Unlock guidance must retain the condition that no launcher operation is running.

### Claim each local calendar date atomically

Use a migration-specific hidden date-stamped file directly inside the existing legacy deployment, for example `.launcher-path-migration-reminder-2026-11-17`. Its name is the persisted reminder date, so no general settings schema or separate lock is needed. Claim it with exclusive creation; an existing regular file for today's or a later local date suppresses the CTA, including after clock rollback. This refines the proposed single timestamp file by avoiding a read-then-write race.

Do not recreate a missing deployment directory to store a marker. An absent or malformed marker date is not today's claim. Remove only this migration's older valid date markers after claiming today, and clean its markers from the destination after a successful move. Unrelated dotfiles remain untouched. A concurrent migration may make the old directory disappear; handle that as benign and preserve the requested command's outcome.

Persistence errors permit best-effort guidance and debug logging while leaving command status unchanged. A normal writable directory provides the once-per-day guarantee; read-only or damaged storage cannot provide durable suppression. A process killed after claiming but before writing stderr can consume today's reminder; this bounded at-most-once trade-off avoids stale locks and an indefinite wait.

### Respect terminal presentation at the right phase

Startup must still migrate before dynamic flag registration. Collect deferred results there and present their CTAs after Cobra has parsed output flags, before command-specific work, using the established CTA visibility policy and stderr presentation. Claim the date only when the CTA is eligible for text presentation. Cover a first JSON invocation followed by text on the same day; the text invocation must still get its reminder.

Use ordinary selected-directory notices for named/default legacy selection. The daily CTA is the single owner of migration guidance. Help and completion retain their existing startup bypass. Preserve existing error-path terminal flushing, and verify when the claim occurs relative to writing the CTA so pre-dispatch failures cannot consume a reminder that was never eligible to print.

### Keep the migration removable

Use dedicated migration files in the command package for startup moves/results, legacy selection/redirection/listing adapters, and reminder persistence/presentation. Move `Legacy*` source resolvers into a migration-specific file in `internal/launcherpaths`; keep permanent destination resolution in `launcherpaths.go`. Use named, small hooks from startup, resolution and listing rather than inline migration policy in those consumers. Do not add a dependency from permanent path resolution back to migration.

Place migration-focused tests beside those files. Document the removal checklist in the migration design or a short contributor note: remove the startup hook and migration files, replace the resolution/listing adapters with current-root operations, remove legacy source helpers and migration tests/specifications/docs. Keep reusable path comparison helpers only if their surviving callers need them. No general plugin interface or registry is warranted for one migration.

## Risks / Trade-offs

- Calendar dates differ from rolling 24 hours: the requirement uses the host's local date. Test midnight, time-zone offsets and clock rollback explicitly; a claim for the current date suppresses repeats.
- Concurrent invocations and midnight can leave a small number of older markers: clean only dates older than the invocation's date, preserving newer claims. Tests must use separate processes for the atomic claim contract.
- General deployment locking is already separate and may be in use: reminder persistence must never acquire or alter that lock.
- Invalid or unwritable storage can cause repeated best-effort CTAs: keep the requested operation usable and expose persistence trouble through debug logging.

## Migration Plan

Implement on `nh.daily-migration-cta_SPOT-34071`, based on `nh.macos-launcher-paths_SPOT-32510`. The first eligible invocation establishes its date marker; no settings migration is needed. After each successful deployment move, retire reminder state. Keep this work in a separate PR, with independently passing checks. Archive completed OpenSpec artifacts before committing them separately from implementation.

## Open Questions

None. The exact helper signatures and CTA wording are implementation details subject to the stated output and lifecycle contracts.
