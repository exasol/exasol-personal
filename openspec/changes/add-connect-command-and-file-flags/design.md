## Context

`exasol connect` (`cmd/exasol/connect.go`) populates a `connect.Opts` and calls `deploy.Connect`, which resolves connection info and calls `connect.Connect` (`internal/connect/connect.go`). `connect.Connect` opens the database, builds a result printer (table or JSON), and then drives input through `RunShellWithOpts`. The shell reads lines via a readline-backed `LineReader`, buffers them, and splits on `;` with `splitSemicolonTerminatedStatements` — a quote- and comment-aware splitter that already exists in `internal/connect/shell.go`. Each statement flows through one `ProcessInputFunc` callback that execs the SQL and prints the result.

The statement-execution callback and the splitter are exactly what `--command`/`--file` need; only the *source* of the SQL and the *driver loop* differ.

## Goals / Non-Goals

**Goals:**
- Let users and agents pass SQL directly via `-c`/`--command` and `-f`/`--file`, running non-interactively and exiting.
- Reuse the existing splitter and exec/print pipeline so output and SQL semantics match the interactive shell.
- Leave the default interactive/stdin path untouched.

**Non-Goals:**
- No new SQL parsing or statement-splitting logic.
- No multi-statement transaction semantics beyond what the database already provides.
- No changes to authentication, connection setup, or the deployment directory contract.
- No support for reading the script from `-` (stdin) — stdin already works without a flag.

## Decisions

**Reuse `splitSemicolonTerminatedStatements` for `-c`/`-f` input.** Both flags supply a complete SQL string (the flag value, or the file contents). We feed that string to the existing splitter and execute the returned statements in order via the same callback `connect.Connect` already builds. Any non-empty remainder after the final `;` is executed as a trailing statement, matching the interactive shell's EOF handling. Alternative — a separate parser — rejected: it would risk diverging from interactive behavior.

**Branch in `connect.Connect` on a new input source, not in the shell.** Add `Command string` and `File string` to `connect.Opts`. After opening the DB and building the printer, if a non-interactive source is set, run the statements directly and return; otherwise fall through to `RunShellWithOpts` as today. This keeps the readline/shell machinery out of the non-interactive path (no history file, no exit hint, no TTY detection).

**Validate mutual exclusivity at the CLI layer.** `cmd/exasol/connect.go` checks `cmd.Flags().Changed("command")` and `Changed("file")` in `RunE` and returns an error before calling `deploy.Connect`, so we fail fast without touching the database. Cobra marks the flags with `MarkFlagsMutuallyExclusive` as the primary guard; the explicit check keeps a clear error message.

**Stop on first error in non-interactive mode (exit non-zero).** The interactive shell logs each statement error and continues, which suits a REPL. For `-c`/`-f`, scripts and agents need a reliable failure signal, so the non-interactive runner stops at the first failing statement and returns the error, which propagates to a non-zero process exit. Alternative — continue-and-aggregate — rejected as the default because partial success is harder for callers to reason about; can be revisited if a `--continue-on-error` flag is later requested.

**Read the file with `os.ReadFile` in the connect layer.** A missing/unreadable file returns a wrapped error before any DB work, giving a clear message and non-zero exit.

## Risks / Trade-offs

- **Behavioral divergence: stop-on-error vs. interactive continue** → Documented explicitly in the spec and README; the two modes serve different audiences (REPL vs. automation).
- **Large script files read fully into memory** → Acceptable; connect scripts are small, and the interactive path already buffers. Streaming can be added later if needed.
- **`-c` shadowing a future global flag** → `-c`/`-f` are scoped to the `connect` subcommand only; verify no existing `connect` short flag collides (current: `-u`, `-p`, `-k`, `-j`).

## Open Questions

- Should `--json` be the implied default when `-c`/`-f` is used by a non-TTY caller? Current decision: no — keep the existing `--json` opt-in so behavior is predictable regardless of input source.
