## Why

`exasol connect` splits SQL input into statements on `;` (quote- and comment-aware). A
`CREATE ... SCRIPT` / `CREATE ... FUNCTION` body is program code (Java/R, and some Python)
where `;` is ordinary syntax (e.g. `return x * 2.0;`). Splitting on the first body `;`
truncated the definition and sent an incomplete statement to the database, producing errors
like `syntax error, unexpected '}'`. This blocked creating/testing Java and R UDFs through
`exasol connect` (`-c`/`-f` and interactive) — a core part of the SLC workflow.

We adopt the EXAplus rule for the terminator ([docs](https://docs.exasol.com/db/latest/connect_exasol/sql_clients/exaplus_cli/exaplus_cli.htm)): script and function definitions end with a `/`
on a line by itself (not `;`), with body semicolons ignored — keeping `exasol connect`
consistent with the client users already rely on for scripts.

## What Changes

- Script/function definitions terminate on a line containing only `/`, not `;`; semicolons
  inside a script body are no longer statement terminators.
- Detection of script DDL is whitespace- and comment-aware; a `CREATE` statement whose only
  `SCRIPT`/`FUNCTION` occurrence is inside a comment is still treated as a normal statement.
- An unterminated script (no `/` yet) is buffered until `/` arrives, and any buffered
  remainder is flushed as a single statement at end of input.
- All other statements continue to split on `;` exactly as before (no behavior change).

## Impact

- `internal/connect/shell.go`: statement splitter (`findStatementTerminator` dispatch,
  `findSemicolonTerminator`, `findScriptTerminator`, `looksLikeScriptDDL`).
- Backward compatible: non-script input splits byte-for-byte as before.

## Capabilities

### Modified Capabilities

- `connect-sql-input`: adds the script/function `/`-terminator rule to the shared splitting
  behavior used by interactive input, `--command`, and `--file`.
