## 1. Query Result Contract

- [x] 1.1 Extend `internal/connect/types.QueryResulter` with a typed row accessor for JSON-compatible values
- [x] 1.2 Update `internal/connect/exasol.QueryResult` to store both typed values and display string rows
- [x] 1.3 Keep non-resultset and import-query results returning empty typed and display rows

## 2. Driver Value Collection

- [x] 2.1 Update `collectRows` to preserve driver values for JSON output while still building `Rows() [][]string`
- [x] 2.2 Convert SQL `NULL` driver values to typed `nil` and display strings compatible with existing table output
- [x] 2.3 Preserve numeric and boolean driver values as JSON-native typed values
- [x] 2.4 Convert date, timestamp, byte slice, and other non-JSON-native driver values to stable strings for JSON output

## 3. JSON Rendering

- [x] 3.1 Change `jsonQueryResult.Rows` to use typed row values instead of string rows
- [x] 3.2 Configure the JSON encoder to avoid unnecessary HTML escaping
- [x] 3.3 Confirm truncation notices, version messages, prompts, and exit hints stay off stdout for non-interactive JSON output
- [x] 3.4 Keep table rendering on `Rows() [][]string` so existing non-JSON output remains compatible

## 4. Tests

- [x] 4.1 Add unit coverage for JSON numbers, booleans, strings, dates or timestamps, nulls, and HTML-sensitive strings
- [x] 4.2 Add unit coverage for `collectRows` preserving typed values and display strings
- [x] 4.3 Update existing test doubles and expectations for the expanded `QueryResulter` contract
- [x] 4.4 Run focused Go tests for `internal/connect/...`

## 5. Validation

- [x] 5.1 Run `task fmt`
- [x] 5.2 Run `task tests-unit`
- [x] 5.3 Run `task lint`
- [x] 5.4 Run `task all` if the focused validation passes and time permits
