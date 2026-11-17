## 1. Isolated migration compatibility

- [x] 1.1 Isolate released-layout source paths, migration execution/results, legacy selection/redirection/listing adapters and focused compatibility tests in migration-specific files with small call sites; retain current behavior and document the later removal checklist. Commit: `refactor: Isolate legacy launcher layout compatibility`.

## 2. Daily migration CTA

- [x] 2.1 Add atomic per-deployment local-date reminder claims and cleanup; route structured deferrals through the command CTA policy after output flags are known; use ordinary selected-directory notices; preserve operational diagnostics. Add Go and launcher behavioral coverage, user documentation and the net unreleased changelog update together. Commit: `fix: Show deferred migration guidance once per day`.

## 3. Validation and review

- [x] 3.1 Verify each implementation commit independently with its relevant Go and launcher tests. Cover first/repeated/day-change reminders, separate deployments, JSON-before-text, redirected text output, help/completion bypass, failure-path output flushing, running/locked/unknown states, collision diagnostics, missing/corrupt/unwritable markers, quoted paths, time-zone offsets and clock rollback, marker cleanup and successful migration after a reminder. Validation only; no commit.
- [x] 3.2 Exercise the daily claim with separate concurrent processes and temporary directories, including old-directory disappearance during migration; run Go race checks on focused state handling. Validate that reminder recording never changes deployment locks. Validation only; no commit.
- [x] 3.3 Run repository-required formatting, lint, `task all` and affected documentation checks. Run meaningful native platform checks where available and cross-compile affected packages otherwise. Keep fixes with their corresponding implementation commit. Validation only; no commit.
- [x] 3.4 Obtain independent code review and evaluate this changed CLI/documentation flow through supported commands. Resolve or disposition findings; verify the documented removal surface against the final diff. Validation only; no commit.

## Delivery sequence

After implementation and verification, sync and archive this change. Commit the main specifications and archived OpenSpec artifacts separately as `docs(openspec): Record daily migration guidance`. Then publish the separate PR against the macOS path branch with SPOT-34071 and completed verification through the delivery workflow.
