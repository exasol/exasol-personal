## Why

`exasol connect` fails on any query whose result exceeds the Exasol websocket inline limit (~1,000 rows), e.g. `SELECT * FROM products` on the sample data fails with `ERR the result set doesn't have expected number of columns: expected=1000000 got=0`. The launcher executes queries via the driver's raw `SimpleExec` and parses only the inline response; for larger results the server returns a result-set handle with no inline data, which the launcher never fetches. A secondary bug makes the error message nonsensical: it prints the row count where the column count belongs.

Simply fetching and printing every row (1M+) is also undesirable: it floods the terminal, transfers and buffers hundreds of MB, and isn't what a human typing `SELECT *` wants to see. Because the server reports the total row count up front via the result-set handle, the client can fetch only what it will display.

## What Changes

- `exasol connect` retrieves complete results for queries of any size by fetching result-set handles in chunks (using the driver's built-in fetch mechanism) instead of failing.
- **Interactive (TTY) sessions** cap output at a default preview limit (100 rows). When more rows are available than were shown, a footer is written to **stderr** (so it never corrupts output) reporting how many rows were shown and how to see all of them. Fetching stops once the cap is reached, so truncated rows are never transferred.
- **Non-interactive sessions** (stdin / piped) return the full result set by default, matching `psql`/`mysql` — automation must receive exactly what it queried, never a silently truncated set.
- A new `--max-rows N` flag overrides the limit in either mode; `--max-rows 0` means unlimited.
- Result-set handles are closed after consumption so the server releases resources.
- The misleading error message (row count printed as expected column count) is removed along with the manual inline-only parsing it guarded.
- No change to statement splitting, output formatting (table/JSON), or the `IMPORT ... FROM LOCAL CSV` special case.

## Capabilities

### New Capabilities
- `connect-result-retrieval`: how `exasol connect` retrieves and bounds query results — inline results, chunked fetching via result-set handles, the interactive preview cap and `--max-rows` override, the stderr truncation footer, handle cleanup, and non-resultset statements (e.g. `OPEN SCHEMA`).

### Modified Capabilities
<!-- None: no existing spec covers result retrieval. -->

## Impact

- `internal/connect/exasol/types`: extend the `ExasolConnector` interface with the driver's `QueryContext` (regenerate the counterfeiter fake).
- `internal/connect/exasol/database.go`: rewrite `Exec` to consume `driver.Rows` (which fetches chunks transparently), bounded by a row limit, instead of parsing the raw `SimpleExec` response.
- `internal/connect/exasol/query_result.go`: manual response parsing and its swapped-variable error message become obsolete.
- `internal/connect/connect.go`: resolve the effective row limit (interactive default vs. unlimited vs. explicit `--max-rows`), thread it into execution, and emit the truncation footer to stderr.
- `cmd/exasol/connect.go`: register the `--max-rows` flag on `connect.Opts`.
- Tests in `internal/connect/...` updated for the new execution path, including chunked (multi-fetch) and truncation scenarios.
- No new dependencies (`exasol-driver-go` already provides everything). No output format changes.
