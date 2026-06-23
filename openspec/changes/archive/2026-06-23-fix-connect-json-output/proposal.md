## Why

`exasol connect --json` currently serializes every SQL value as a string and renders SQL `NULL` as `"<nil>"`, which makes the output unsafe for scripts, `jq` pipelines, APIs, monitoring, and agent workflows. The JSON mode needs a stable machine-readable contract that preserves JSON-compatible SQL values.

## What Changes

- Preserve typed query result values through the connect execution path so JSON output can emit native JSON numbers, booleans, strings, and nulls.
- Keep existing table output compatible by rendering the same display strings it uses today.
- Emit SQL `NULL` as JSON `null` in `--json` output.
- Emit JSON-compatible numeric and boolean values as JSON numbers and booleans where the driver value can be represented safely.
- Encode JSON output without unnecessary HTML escaping of ordinary result strings.
- Keep `--json` stdout clean in non-interactive execution; diagnostic and truncation messages stay off stdout.
- Add focused tests for numeric, boolean, string, date or timestamp, null, and HTML-sensitive string behavior.

## Capabilities

### New Capabilities

- `connect-json-output`: JSON result formatting contract for `exasol connect --json`, including typed values, null handling, string escaping, and stdout cleanliness.

### Modified Capabilities

<!-- None. Existing connect input requirements remain unchanged. -->

## Impact

- `internal/connect/types`: extend the query result contract so JSON rendering can access typed row values while table rendering can continue using display strings.
- `internal/connect/exasol`: preserve driver values when collecting rows and expose both typed values and string-rendered rows.
- `internal/connect/connect.go`: render JSON from typed rows, disable HTML escaping, and preserve stdout/stderr separation.
- Tests in `internal/connect/...` cover the JSON contract and table compatibility.
- No new dependencies, no CLI flag changes, and no change to default non-JSON output mode.
