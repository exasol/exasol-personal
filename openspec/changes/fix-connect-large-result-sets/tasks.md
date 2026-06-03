## 1. Execution path and bounded fetch

- [x] 1.1 Add `QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error)` to `types.ExasolConnector` in `internal/connect/exasol/types/types.go` and regenerate the counterfeiter fake (`task generate`)
- [x] 1.2 Change `Database.Exec` to accept an explicit row limit (`limit int`, 0 = unlimited) and signal truncation back to the caller (return a `truncated bool` or expose it on the result)
- [x] 1.3 Rewrite `Exec` (non-import path) to call `conn.QueryContext(ctx, query, nil)`, read `rows.Columns()`, iterate `rows.Next` building `[][]string` with `fmt.Sprint`, stop once `limit` rows are collected, peek one more `Next` to detect truncation, and `defer rows.Close()`
- [x] 1.4 Keep the `isImportQuery` branch on `conn.Exec` unchanged
- [x] 1.5 Remove `FromResultSet`, `getColumnNames`, `getRows`, and the `ErrNumColumns`/`ErrNumRows` validation from `internal/connect/exasol/query_result.go`, keeping `QueryResult` as the `QueryResulter` data holder
- [x] 1.6 Drop `SimpleExec` from the `ExasolConnector` interface if no callers remain; otherwise leave it and note why

## 2. Limit policy, flag, and footer

- [x] 2.1 Add `MaxRows` to `connect.Opts` and register `--max-rows` on `connectCmd` in `cmd/exasol/connect.go` (default sentinel meaning "unset")
- [x] 2.2 In `connect.Connect`, resolve the effective limit once: explicit `--max-rows` wins; else interactive (`util.IsInteractiveStdin()`) → 100, non-interactive → 0 (unlimited); thread it into per-statement execution
- [x] 2.3 When a statement reports truncation, write a footer (e.g. `showing first N rows (output truncated; use --max-rows 0 to see all)`) to `os.Stderr` — never stdout
- [x] 2.4 Confirm `--max-rows 0` disables the cap and suppresses the footer in all modes

## 3. Tests

- [x] 3.1 Update `internal/connect/exasol/database_test.go` to fake `QueryContext`: small result (columns + rows as strings), non-resultset statement (empty columns, immediate EOF), and a query error
- [x] 3.2 Chunked-result test: a fake `driver.Rows` serving rows across multiple `Next` chunks, asserting all rows returned (unlimited) and `Close` called exactly once
- [x] 3.3 Truncation test: limit=N against a fake with >N rows asserts exactly N collected, `truncated=true`, and no fetching past the peek
- [x] 3.4 Integer DECIMAL rendering: a fake returning `int64(1000000)` renders as `1000000` (not `1e+06`)
- [x] 3.5 Limit-resolution + footer test in the `connect` package: interactive→100, non-interactive→unlimited, `--max-rows` override; footer goes to stderr and only when truncated
- [x] 3.6 Update or remove `query_result_test.go` to match the slimmed-down `QueryResult`
- [x] 3.7 Run `go test ./internal/connect/...` and `task tests-unit`

## 4. Verification and docs

- [ ] 4.1 Manual verification against a live deployment: interactive `OPEN SCHEMA sample; SELECT * FROM products;` shows 100 rows + stderr footer; `--max-rows 0` shows all; piped input returns all by default; `SELECT 1` and `OPEN SCHEMA` still behave
- [x] 4.2 Document `--max-rows` and the interactive preview cap in the README `connect` section
- [x] 4.3 Run `task fmt` and `task lint`; fix any findings
