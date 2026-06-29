## 1. JSON contract and execution metadata

- [x] 1.1 Extend the connect query-result contract with statement metadata needed for JSON output, including statement type and rows affected
- [x] 1.2 Add lightweight SQL statement classification and populate metadata for result-set, DDL, DML, import, and session statements
- [x] 1.3 Add structured SQL error normalization in the connect or database layer so JSON mode can emit machine-readable error details

## 2. Invocation-level JSON rendering

- [x] 2.1 Refactor non-interactive JSON execution to aggregate executed statements into a single invocation document instead of printing one top-level object per statement
- [x] 2.2 Ensure single-statement, multi-statement, and partial-success-plus-error flows all emit one valid JSON document on stdout
- [x] 2.3 Keep table and CSV output behavior unchanged, including stderr-only truncation notices

## 3. Tests and documentation

- [x] 3.1 Update `internal/connect` JSON rendering tests for the new invocation envelope and statement metadata
- [x] 3.2 Add or update database-layer tests covering statement metadata, rows affected, and structured SQL error extraction
- [x] 3.3 Update command/help or spec-facing documentation that describes `exasol connect --json`, then run focused formatting and unit validation
