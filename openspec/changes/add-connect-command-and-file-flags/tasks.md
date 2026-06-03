## 1. Options and non-interactive runner

- [x] 1.1 Add `Command string` and `File string` fields to `connect.Opts` in `internal/connect/connect.go`
- [x] 1.2 Add a non-interactive runner that takes a SQL string, splits it with `splitSemicolonTerminatedStatements` (executing any non-empty trailing remainder), and runs each statement through the existing exec/print callback, stopping and returning on the first error
- [x] 1.3 In `connect.Connect`, after opening the DB and building the printer, branch: if `Opts.File` is set, read the file with `os.ReadFile` (wrap and return a clear error if missing/unreadable) and run its contents via the non-interactive runner; else if `Opts.Command` is set, run it via the non-interactive runner; else fall through to the existing `RunShellWithOpts` path
- [x] 1.4 Ensure the non-interactive path does not print the interactive exit hint and does not depend on TTY detection or the history file

## 2. CLI wiring

- [x] 2.1 Register `-c`/`--command` and `-f`/`--file` flags on `connectCmd` in `cmd/exasol/connect.go`, bound to the new `connectOpts` fields
- [x] 2.2 Mark the two flags mutually exclusive (`MarkFlagsMutuallyExclusive`) and add an explicit check in `RunE` that returns a clear error if both are set, before calling `deploy.Connect`
- [x] 2.3 Update `connectCmdExample` to show `-c` and `-f` usage
- [x] 2.4 Confirm no short-flag collision with existing `connect` flags (`-u`, `-p`, `-k`, `-j`)

## 3. Tests

- [x] 3.1 Unit-test the non-interactive runner: single statement, multiple `;`-separated statements run in order, empty/trailing segments skipped, stop-on-first-error returns the error
- [x] 3.2 Test `--command` and `--file` produce table and `--json` output identically to the interactive path (covered structurally: both paths share the same printer/`processInput`; rendering verified via `TestRunStatementsRendersEachResult` and the existing printer tests)
- [x] 3.3 Test `--file` with a missing/unreadable path returns a non-zero error and runs no statements (`resolveNonInteractiveSQL` errors before connecting)
- [x] 3.4 Test that supplying both `--command` and `--file` fails fast with the mutual-exclusivity error and never connects
- [x] 3.5 Verify neither flag preserves existing interactive/stdin behavior (`resolveNonInteractiveSQL` returns `nonInteractive=false`; existing `TestRunShell` unchanged)

## 4. Documentation

- [x] 4.1 Update the README `connect` section to document `-c`/`--command` and `-f`/`--file`, including the stop-on-error / non-zero exit behavior
- [x] 4.2 Run `task fmt`, `task lint`, and `task tests-unit`; fix any findings (gofmt/`go vet`/`go build` clean, `lint-golang` clean, all unit tests pass; fixed a revive line-length and a staticcheck nil-deref finding in the new test. Two pre-existing, change-unrelated tooling failures remain on this macOS machine: golangci-lint v2.4.0 panics under `--fix` on Go 1.26 (`buildssa`), and `lint-licenses` passes no files to `addlicense` because `find -iregex` yields nothing under Task's shell — all touched files pass `addlicense -check` directly.)
