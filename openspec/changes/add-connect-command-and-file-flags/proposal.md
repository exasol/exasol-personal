## Why

`exasol connect` can only receive SQL interactively or via stdin (pipes, heredocs). Passing a single statement or a script file requires shell plumbing that is awkward for humans and brittle for automation and AI agents. Other SQL clients (e.g. `psql -c`/`-f`, `mysql -e`) expose direct flags for this; `exasol connect` should too.

## What Changes

- Add `-c`/`--command <SQL>` to `exasol connect`: run one or more `;`-separated SQL statements passed as the flag argument, then exit.
- Add `-f`/`--file <path>` to `exasol connect`: read a SQL script from the given file, run its `;`-separated statements, then exit.
- Both flags run non-interactively: no readline shell, no exit hint, no history. The same statement splitting and result printing (table or `--json`) used by the interactive shell apply.
- `-c` and `-f` are mutually exclusive; supplying both is an error. When either is given, stdin is not read.
- No change to existing behavior: with neither flag, `connect` keeps its interactive/stdin behavior.

## Capabilities

### New Capabilities
- `connect-sql-input`: how `exasol connect` accepts SQL input — interactive shell, stdin, and the new `--command`/`--file` non-interactive sources, including precedence, splitting, output, and exit-code semantics.

### Modified Capabilities
<!-- None: there is no existing spec covering connect input behavior. -->

## Impact

- `cmd/exasol/connect.go`: register the two flags and validate mutual exclusivity.
- `internal/connect`: add a non-interactive execution path that splits and runs supplied SQL through the existing exec/print pipeline (reusing `splitSemicolonTerminatedStatements`) instead of the readline shell.
- `internal/deploy/connect.go`: unchanged connection setup; routes to interactive vs. non-interactive based on opts.
- Docs: README `connect` section and `cmd/exasol/connect.go` examples.
- No new dependencies. No changes to the deployment directory or compatibility contract.
