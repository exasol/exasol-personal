## ADDED Requirements

### Requirement: JSON Statement Metadata Distinguishes Statement Outcomes

`exasol connect --json` SHALL emit one statement record per executed SQL statement, and each record SHALL include metadata that distinguishes result-set statements from DDL, DML, and session statements.

#### Scenario: DDL and session statements carry statement metadata

- **WHEN** a non-result statement such as `OPEN SCHEMA`, `CREATE TABLE`, or `COMMIT` succeeds under `exasol connect --json`
- **THEN** its statement record includes a stable `statementType`
- **AND** the record includes `rowsAffected`
- **AND** the record includes `columns` and `rows` as empty arrays when no result set was returned

#### Scenario: Zero-row selects remain distinguishable from non-result statements

- **WHEN** a `SELECT` statement succeeds with zero result rows under `exasol connect --json`
- **THEN** its statement record includes `statementType` identifying it as a query statement
- **AND** its `columns` reflect the query result columns even when `rows` is empty
- **AND** consumers can distinguish it from a DDL or session statement without inferring from empty arrays alone

#### Scenario: Result-set statements keep stable rowsAffected semantics

- **WHEN** a result-set statement succeeds under `exasol connect --json`
- **THEN** its statement record still includes `rowsAffected`
- **AND** `rowsAffected` is `0` when the database driver does not expose affected-row metadata for that statement kind
- **AND** consumers can rely on `statementType`, `columns`, and `rows` to interpret query results

### Requirement: JSON SQL Errors Are Structured

When `--json` is used in non-interactive execution, SQL failures SHALL be emitted as structured JSON rather than plain text.

#### Scenario: Statement failure returns structured error details

- **WHEN** a statement fails during `exasol connect --json` execution
- **THEN** stdout remains a single valid JSON document
- **AND** the document includes a structured error object with `errorCode`, `sqlState`, `message`, `sessionId`, and `position` line or column data when available
- **AND** the error identifies the failing statement record or its position within the invocation

#### Scenario: Earlier successful statements remain represented when a later statement fails

- **WHEN** a multi-statement script succeeds for one or more statements before a later statement fails under `exasol connect --json`
- **THEN** the JSON document includes statement records for the successful statements executed before the failure
- **AND** the JSON document includes the structured error for the failing statement
- **AND** no additional plain-text SQL error is written to stdout

## MODIFIED Requirements

### Requirement: Typed JSON Query Result Values

`exasol connect --json` SHALL emit query result rows within statement records using JSON-compatible value types instead of converting every cell to a string.

#### Scenario: Numeric values are JSON numbers

- **WHEN** a query statement result contains numeric SQL values returned by the database driver as safely representable numeric values
- **THEN** `exasol connect --json` emits those cells as JSON numbers in the statement record row data

#### Scenario: Boolean values are JSON booleans

- **WHEN** a query statement result contains boolean SQL values
- **THEN** `exasol connect --json` emits those cells as JSON booleans in the statement record row data

#### Scenario: Null values are JSON null

- **WHEN** a query statement result contains SQL `NULL` values
- **THEN** `exasol connect --json` emits those cells as JSON `null` in the statement record row data

#### Scenario: Text values remain JSON strings

- **WHEN** a query statement result contains text, date, timestamp, or other non-JSON-native values represented as strings
- **THEN** `exasol connect --json` emits those cells as JSON strings in the statement record row data

### Requirement: JSON Strings Are Not HTML-Escaped Unnecessarily

`exasol connect --json` SHALL encode result strings without unnecessary HTML escaping of ordinary characters.

#### Scenario: HTML-sensitive characters stay readable

- **WHEN** a query result string contains ordinary characters such as `<`, `>`, or `&`
- **THEN** `exasol connect --json` emits those characters in the JSON string without replacing them with Unicode escape sequences solely for HTML safety

### Requirement: JSON Stdout Is Machine-Readable

When `--json` is used in non-interactive execution, `exasol connect` SHALL write exactly one JSON document to stdout for the full invocation.

#### Scenario: Single-statement JSON output uses the invocation envelope

- **WHEN** the user runs `exasol connect --json` non-interactively with one SQL statement from `--command`, `--file`, or piped stdin
- **THEN** stdout contains one valid JSON document for the invocation
- **AND** the document contains exactly one statement record inside the invocation envelope

#### Scenario: Multi-statement JSON output remains parseable

- **WHEN** the user runs `exasol connect --json` non-interactively with multiple SQL statements
- **THEN** stdout contains one valid JSON document for the invocation rather than adjacent top-level JSON objects
- **AND** the document contains one statement record per executed statement in execution order

#### Scenario: Non-interactive JSON output contains no prompts or status text

- **WHEN** the user runs `exasol connect --json` non-interactively with SQL from `--command`, `--file`, or piped stdin
- **THEN** stdout contains only the JSON invocation document
- **AND** prompts, banners, version messages, exit hints, truncation notices, and unstructured SQL errors are not written to stdout

#### Scenario: Interactive JSON output remains statement-oriented

- **WHEN** the user runs `exasol connect --json` interactively in the shell
- **THEN** each executed statement is emitted as its own JSON document
- **AND** the interactive shell does not aggregate multiple statements into one invocation envelope

### Requirement: Non-JSON Output Compatibility

Existing non-JSON query output SHALL continue to render display strings for result cells.

#### Scenario: Pretty table output remains string-rendered

- **WHEN** the user runs `exasol connect` without `--json`
- **THEN** query result cells are rendered through the existing table output path using display strings
