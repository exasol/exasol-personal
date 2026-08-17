## Context

The existing `connect-json-output` work preserved typed cell values, but `connect.Connect` still executes non-interactive scripts one statement at a time and prints each JSON result immediately. That behavior produces invalid concatenated JSON for multi-statement scripts, returns plain CLI errors for SQL failures, and uses the same `{columns:[], rows:[]}` shape for zero-row queries and non-result statements.

The fix crosses the shell runner, connect execution flow, database adapter, and JSON renderer. It must preserve interactive table behavior, keep stdout machine-readable in non-interactive JSON mode, and avoid changing the default non-JSON output contract.

## Goals / Non-Goals

**Goals:**

- Emit one parseable JSON document per non-interactive `exasol connect --json` invocation.
- Include per-statement metadata so result-set, DDL, and session statements are distinguishable.
- Convert SQL failures in JSON mode into structured error objects without losing available database details.
- Keep existing non-JSON output behavior unchanged.
- Cover the new contract with focused unit tests and JSON rendering tests.

**Non-Goals:**

- No change to the default table or CSV output contracts.
- No new SQL parsing engine or deep semantic SQL classification beyond lightweight statement-kind detection.
- No change to the Exasol SQL type system or typed-cell behavior already delivered.
- No new NDJSON mode in this change; the selected contract is a single invocation envelope.

## Decisions

**Use an invocation envelope for non-interactive JSON output.** Instead of printing one top-level object per statement, `connect.Connect` will collect non-interactive statement outcomes and encode a single document such as `{"statements":[...]}` on success, or `{"statements":[..., {"error": ...}]}` when a statement fails after partial execution. The structured error is attached to the failing statement record so consumers can identify the failed statement without correlating a separate top-level field. Interactive `--json` output is intentionally left on the existing per-statement shape in this change so the shell workflow and prompt-driven execution model stay unchanged. Alternative: add NDJSON or delimiters. Rejected because the user story explicitly wants whole-output `JSON.parse` compatibility and no custom stream splitter.

**Represent each statement explicitly.** Add a statement record with `statement`, `statementType`, `rowsAffected`, `columns`, `rows`, and `truncated`. Result-set statements keep typed row values; DDL/session statements use empty columns/rows plus metadata so they no longer look identical to zero-row `SELECT`s. Alternative: add metadata beside the old top-level shape only for non-empty results. Rejected because consumers need uniform per-statement records.

**Keep JSON aggregation in the connect layer.** The database adapter continues to execute one statement at a time, but returns richer execution metadata through `QueryResulter`. The connect layer owns script splitting and invocation-level envelope assembly. Alternative: move multi-statement aggregation into the database package. Rejected because splitting and non-interactive control flow already live in `internal/connect`.

**Classify statement types with lightweight SQL inspection.** The launcher will infer a coarse statement type from the trimmed SQL text by examining the first keyword pair when needed, covering categories such as `SELECT`, `WITH`, `INSERT`, `UPDATE`, `DELETE`, `MERGE`, `IMPORT`, `EXPORT`, `CREATE`, `ALTER`, `DROP`, `TRUNCATE`, `OPEN_SCHEMA`, `CLOSE_SCHEMA`, `SET`, `COMMIT`, `ROLLBACK`, `GRANT`, and `REVOKE`. Alternative: rely on database driver metadata. Rejected because the current interfaces do not expose statement class and the story only needs stable coarse metadata.

**Capture rows affected from the execution path that knows it.** Non-result statements and import-style executions will use `driver.Result.RowsAffected()` when available. Result-set statements still include `rowsAffected`, but use `0` when the driver does not expose affected-row semantics for that statement kind. This keeps DDL/session statements distinguishable without inventing an affected-row count for every query kind, and it keeps the JSON shape uniform for consumers.

**Normalize SQL errors into structured JSON details.** Introduce a small structured error type in the connect domain that records `errorCode`, `sqlState`, `message`, `sessionId`, and optional line/column coordinates. The database adapter will inspect driver errors, extract those fields when present, and wrap unknown errors with at least the message. The session id is fetched lazily on the first execution error that needs enrichment instead of during every successful connect, so non-JSON and success-only sessions do not pay an extra round-trip. Non-JSON execution still returns ordinary Go errors so the CLI’s text behavior remains unchanged.

## Risks / Trade-offs

- **Statement classification is heuristic** -> Limit the contract to coarse stable types and cover representative SQL forms in tests.
- **Driver error shapes may vary** -> Implement best-effort extraction with graceful fallback to message-only structured errors.
- **Rows affected may be unavailable for some statements** -> Treat it as metadata populated when the driver provides it and keep statement type as the primary discriminator.
- **Invocation-level JSON changes the single-statement shape** -> Update command docs, tests, and the capability spec so consumers have one consistent contract.
