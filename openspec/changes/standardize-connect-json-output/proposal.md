## Why

`exasol connect --json` is still awkward for automation after the value-typing work because multi-statement execution emits back-to-back top-level objects, SQL failures fall back to unstructured text, and DDL or session statements are indistinguishable from zero-row queries. Agent and integration consumers need one stable invocation-level JSON contract they can parse without custom stream splitting or regex-based error handling.

## What Changes

- Wrap non-interactive `exasol connect --json` results in a single invocation document that remains parseable for single statements, multi-statement scripts, and failures.
- Add per-statement metadata to JSON output, including statement type, rows affected, truncation, and result-set data when present.
- Emit structured SQL errors in JSON mode, including database error code, SQL state, message, session identifier, and line or column position when available.
- Keep non-JSON output unchanged and continue preserving typed JSON cell values introduced by the earlier `connect-json-output` work.
- Add focused tests covering single-statement, multi-statement, DDL or session statements, and structured SQL error behavior.

## Capabilities

### New Capabilities
<!-- None. -->

### Modified Capabilities
- `connect-json-output`: expand the JSON result contract from per-statement result objects to a structured invocation envelope with statement metadata and structured SQL errors

## Impact

- `internal/connect`: aggregate non-interactive JSON execution into one document and render statement-level metadata.
- `internal/connect/exasol`: classify executed statements, capture rows affected for non-result statements, and normalize SQL driver errors into structured JSON data.
- `cmd/exasol`: refresh command help and examples if the emitted JSON shape changes materially.
- Tests in `internal/connect/...` and command coverage need updates for the new JSON contract.
