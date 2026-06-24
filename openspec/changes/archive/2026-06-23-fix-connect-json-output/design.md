## Context

`exasol connect` currently materializes query results in `internal/connect/exasol.Database.Exec` as `[][]string`. Each `driver.Value` returned by the database driver is converted immediately with `fmt.Sprint`, so type information is lost before the JSON renderer sees the data. That makes JSON output serialize numbers and booleans as strings and turns SQL `NULL` into the string `"<nil>"`.

The table renderer depends on string rows, so the fix must preserve the existing display-oriented path while adding a typed path for JSON output. The existing stdout/stderr split already keeps truncation notices on stderr; the JSON encoder still needs to avoid HTML escaping.

## Goals / Non-Goals

**Goals:**

- Preserve JSON-compatible SQL value types for `exasol connect --json`.
- Emit SQL `NULL` values as JSON `null`.
- Keep non-JSON table output compatible with the current rendered strings.
- Keep `--json` stdout parseable and free of non-JSON status text in non-interactive execution.
- Cover typed JSON output behavior with focused unit tests.

**Non-Goals:**

- No new CLI flags or output mode names.
- No public REST API or broader SQL type-system definition.
- No change to statement splitting, query execution order, truncation policy, or default pretty table output.
- No deployment-level end-to-end test requirement for this change; those tests require live infrastructure and are reserved for release validation.

## Decisions

**Add typed rows beside string rows.** Extend `QueryResulter` with a typed row accessor such as `Values() [][]any`, and update `QueryResult` to store both typed values and display strings. `Rows() [][]string` remains available for table output and existing tests. Alternative: change `Rows()` to `[][]any` and convert table rows at the printer boundary. Rejected because it would make the existing display contract noisier and broaden the change surface.

**Normalize driver values while collecting rows.** When `collectRows` receives a `driver.Value`, keep JSON-native values (`nil`, strings, booleans, integer and floating numeric types) as typed values and derive string rows separately with `fmt.Sprint`. For byte slices, timestamps, and other driver-supported values, keep JSON output stable by converting to strings. Alternative: defer all conversion to JSON rendering. Rejected because the database package owns the driver boundary and can keep the general query result contract consistent.

**Keep precision conservative.** JSON numbers are emitted for numeric Go driver values already returned as numeric types. Values that arrive as strings stay strings. This avoids inventing SQL precision rules in the launcher and matches the Jira criterion that numbers are emitted as JSON numbers where the selected JSON contract can represent them safely.

**Disable HTML escaping on the JSON encoder.** Use `encoder.SetEscapeHTML(false)` for connect JSON output so ordinary strings containing characters such as `<`, `>`, and `&` are not unnecessarily escaped. Alternative: post-process encoded JSON. Rejected because the standard encoder exposes the needed setting directly.

**Continue writing diagnostics away from stdout.** Version/exit hints are already TTY-only and written to stderr. Truncation footer also remains stderr-only. This preserves parseable stdout for non-interactive `--json` use.

## Risks / Trade-offs

- **Interface expansion affects fakes and test doubles** -> Regenerate or manually update the counterfeiter fake and local stubs.
- **Exact SQL DECIMAL precision semantics remain driver-dependent** -> Only emit numeric JSON values for numeric driver values; do not parse string values into numbers.
- **Date and timestamp formatting depends on driver value shape** -> Convert non-JSON-native time values to strings, and test the selected behavior at the row collector boundary.
- **Deployment-level JSON behavior is not tested in CI** -> Unit tests cover the contract without requiring cloud resources; deployment tests can be used manually when a live deployment is available.
