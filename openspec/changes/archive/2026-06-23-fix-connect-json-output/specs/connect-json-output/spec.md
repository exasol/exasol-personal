## ADDED Requirements

### Requirement: Typed JSON Query Result Values

`exasol connect --json` SHALL emit query result rows using JSON-compatible value types instead of converting every cell to a string.

#### Scenario: Numeric values are JSON numbers

- **WHEN** a query result contains numeric SQL values returned by the database driver as safely representable numeric values
- **THEN** `exasol connect --json` emits those cells as JSON numbers

#### Scenario: Boolean values are JSON booleans

- **WHEN** a query result contains boolean SQL values
- **THEN** `exasol connect --json` emits those cells as JSON booleans

#### Scenario: Null values are JSON null

- **WHEN** a query result contains SQL `NULL` values
- **THEN** `exasol connect --json` emits those cells as JSON `null`

#### Scenario: Text values remain JSON strings

- **WHEN** a query result contains text, date, timestamp, or other non-JSON-native values represented as strings
- **THEN** `exasol connect --json` emits those cells as JSON strings

### Requirement: JSON Strings Are Not HTML-Escaped Unnecessarily

`exasol connect --json` SHALL encode result strings without unnecessary HTML escaping of ordinary characters.

#### Scenario: HTML-sensitive characters stay readable

- **WHEN** a query result string contains ordinary characters such as `<`, `>`, or `&`
- **THEN** `exasol connect --json` emits those characters in the JSON string without replacing them with Unicode escape sequences solely for HTML safety

### Requirement: JSON Stdout Is Machine-Readable

When `--json` is used in non-interactive execution, `exasol connect` SHALL write only JSON result documents to stdout.

#### Scenario: Non-interactive JSON output contains no prompts or status text

- **WHEN** the user runs `exasol connect --json` non-interactively with SQL from `--command`, `--file`, or piped stdin
- **THEN** stdout contains only the JSON result output for executed statements
- **AND** prompts, banners, version messages, exit hints, and truncation notices are not written to stdout

### Requirement: Non-JSON Output Compatibility

Existing non-JSON query output SHALL continue to render display strings for result cells.

#### Scenario: Pretty table output remains string-rendered

- **WHEN** the user runs `exasol connect` without `--json`
- **THEN** query result cells are rendered through the existing table output path using display strings
