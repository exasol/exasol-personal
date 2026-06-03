## Context

`internal/connect/exasol/database.go:Exec` runs queries via the driver's raw `SimpleExec` and hand-parses the first websocket response (`query_result.go:FromResultSet`). The Exasol websocket protocol returns at most ~1,000 rows inline; larger results come back as `{resultSetHandle, numRows, numRowsInMessage: 0, data: []}` and the client must issue `fetch` commands against the handle. The launcher never fetches, so the shape validation fails — with a message that additionally prints `numRows` where `numColumns` belongs (`query_result.go:63`).

The driver (`exasol-driver-go v1.0.16`) already solves the fetching: `connection.QueryResults` implements `driver.Rows`, transparently calling `fetchNextRowChunk` in `Next()` and closing the handle in `Close()`. Interactive vs. non-interactive is already distinguished elsewhere via `util.IsInteractiveStdin()`.

## Goals / Non-Goals

**Goals:**
- Queries of any result size work in `exasol connect`.
- Interactive sessions show a bounded preview (default 100 rows) without flooding the terminal or transferring unshown rows; non-interactive sessions return everything by default.
- A `--max-rows N` flag overrides the bound in either mode (`0` = unlimited).
- Result-set handles are closed after consumption.
- The `QueryResulter` contract (`ColumnNames()`, `Rows() [][]string`) and output formatting stay unchanged.

**Non-Goals:**
- No streaming to the printer; displayed rows are materialized in memory (the table writer needs them all for column sizing). The point of the cap is to bound *what is displayed and fetched*, not to stream.
- No pager. We bound the fetch instead of relying on `less`/`more.com` (the latter is the weak Windows default and still requires buffering everything).
- No change to the `IMPORT ... FROM LOCAL CSV` path, which stays on `conn.Exec`.

## Decisions

**Fetch through the driver's `driver.Rows`, bounded by a row limit.** Extend `types.ExasolConnector` with `QueryContext(ctx, query string, args []driver.NamedValue) (driver.Rows, error)` — already implemented by the driver's `*Connection`. With empty args it routes through `executeSimpleWithRows` → `SimpleExec` + `ToRow`, i.e. the same execution semantics as today but wrapped in `QueryResults`, which handles chunked fetching and handle cleanup. `Exec` reads `rows.Columns()`, then loops `rows.Next(dest)` building `[][]string` via `fmt.Sprint`, and **stops once it has collected the effective limit**, then `defer rows.Close()` (which closes the server-side handle, including on early stop). Alternative — re-implement fetch loops against driver internals — rejected: duplicates maintained upstream logic (fetch sizing, chunk pointers, handle close).

**Truncation is detected by a peek, not by a reported total.** `driver.Rows` exposes only `Columns()`/`Next()`/`Close()`; `QueryResults.data.NumRows` (the true total) is unexported with no accessor. So after collecting N rows under a positive limit, `Exec` attempts one more `Next()`: `io.EOF` means the result was exactly N (or fewer) — not truncated; a returned row means more exist — truncated (the peeked row is discarded). This needs no total and uses only the public driver API. **Consequence:** the truncation footer cannot state an exact total (`"100 of 1000000"`); it states the count shown plus that output was truncated. Getting the exact total would require either a second execution (unacceptable: side effects, cost) or reimplementing the fetch protocol — see Open Questions.

**Effective limit resolution lives in `connect.Connect`, not in `Exec`.** `Exec` takes an explicit `limit int` (0 = unlimited) so it stays policy-free and testable. `connect.Connect` computes the limit once: explicit `--max-rows` wins; otherwise interactive (`util.IsInteractiveStdin()`) → 100, non-interactive → 0 (unlimited). It passes the limit into the per-statement execution and, when a statement reports truncation, writes the footer to `os.Stderr` (keeping `--json` on stdout valid). `Exec` signals truncation back to `Connect` — e.g. `Exec` returns the `QueryResulter` plus a `truncated bool`, or the `QueryResulter` carries a `Truncated()` flag.

**Non-query statements keep working through the same path.** Verified against driver source: a `rowCount`-type response unmarshals into an empty result set (`NumRows: 0`), so `Next` returns `io.EOF` immediately and `Columns()` is empty — equivalent to today's empty `QueryResult`. The old "Query doesn't work for OPEN SCHEMA" note applied to the `database/sql` layer, not to `QueryContext` on the raw connection.

**Delete the obsolete manual parsing.** `query_result.go`'s `FromResultSet`/`getColumnNames`/`getRows`, the `ErrNumColumns`/`ErrNumRows` shape validation, and the swapped-variable error message are removed; `QueryResult` remains the plain `QueryResulter` data holder (plus a truncation flag if that representation is chosen). The "Truthful retrieval error messages" requirement is satisfied by deletion — the buggy message ceases to exist; any residual shape check uses the correct dimension.

**Value rendering stays `fmt.Sprint`, with one accepted improvement.** The driver's `convertValue` returns `int64` for scale-0 DECIMALs, so integer columns render as `1000000` instead of the float64 artifact `1e+06` the old raw-JSON path produced. Strict improvement; NULLs render as before.

## Risks / Trade-offs

- **Footer lacks an exact total** → Accepted given the driver API; "showing first N rows (truncated)" is honest and sufficient to prompt `--max-rows 0`.
- **Uncapped non-interactive `SELECT *` still materializes the full result (hundreds of MB)** → Accepted and intended: automation must receive the complete set; bounding it silently would corrupt results. Memory cost mirrors psql/mysql.
- **`fetchNextRowChunk` uses `context.Background()` internally (driver limitation)** → cancellation mid-fetch isn't immediate; same blocking behavior as long driver calls today, not worked around here.
- **Fake regeneration churn (`counterfeiter`)** → mechanical; `task generate` covers it.

## Open Questions

- Worth a follow-up to surface the exact total in the footer? It would need an upstream driver accessor on `QueryResults` (or a `RowsColumnType`-style extension). Out of scope here; the honest "truncated" footer ships now.
- Remove `SimpleExec` from `ExasolConnector` once `Exec` no longer calls it? Default: remove if no callers remain, keeping the interface minimal.
