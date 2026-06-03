## ADDED Requirements

### Requirement: Retrieval of large result sets

`exasol connect` SHALL retrieve query results regardless of size. When the database returns a result-set handle rather than complete inline data, the client SHALL fetch the rows it intends to display in chunks until either all rows are retrieved or the effective row limit is reached, and present them exactly as if they had arrived inline. The client SHALL NOT fetch rows beyond the effective row limit.

#### Scenario: Query result exceeds the inline limit

- **WHEN** the user executes a query returning more rows than the websocket inline limit (e.g. `SELECT * FROM products`) with no effective row limit
- **THEN** all rows are retrieved via chunked fetching and printed
- **AND** no error is raised

#### Scenario: Small result set still works

- **WHEN** the user executes a query whose full result arrives inline (at or below the inline limit)
- **THEN** the rows are printed as before, with no additional fetch round-trips

### Requirement: Interactive preview cap

When `exasol connect` runs interactively (standard input is a terminal) and no explicit row limit is given, it SHALL limit displayed rows to a default preview limit of 100. When more rows are available than were displayed, the client SHALL write a footer to standard error reporting the number of rows shown, indicating that the output was truncated, and indicating how to retrieve all rows. The footer SHALL NOT be written to standard output.

#### Scenario: Interactive query exceeds the preview cap

- **WHEN** an interactive user runs a query returning 1,000,000 rows with no `--max-rows` flag
- **THEN** only the first 100 rows are fetched and printed
- **AND** a footer such as `showing first 100 rows (output truncated; use --max-rows 0 to see all)` is written to standard error

#### Scenario: Interactive query within the preview cap

- **WHEN** an interactive user runs a query returning fewer rows than the preview cap
- **THEN** all rows are printed
- **AND** no truncation footer is written

### Requirement: Non-interactive results are not capped by default

When `exasol connect` runs non-interactively (standard input is not a terminal, e.g. a pipe or redirected input) and no explicit row limit is given, it SHALL return the complete result set. It SHALL NOT silently truncate non-interactive output.

#### Scenario: Piped query returns everything

- **WHEN** SQL is piped into `exasol connect` and the query returns more rows than the preview cap, with no `--max-rows` flag
- **THEN** the complete result set is printed with no truncation footer

### Requirement: Explicit row limit override

`exasol connect` SHALL accept a `--max-rows N` flag that sets the effective row limit in any mode. A value of `0` SHALL mean unlimited. A positive value SHALL cap displayed rows to N and, when the result is truncated, behave like the preview cap (fetching stops at N; the truncation footer is written to standard error).

#### Scenario: Override caps an otherwise-unlimited mode

- **WHEN** SQL is piped into `exasol connect --max-rows 10` for a query returning 1,000,000 rows
- **THEN** only 10 rows are fetched and printed
- **AND** a truncation footer is written to standard error

#### Scenario: Override lifts the interactive cap

- **WHEN** an interactive user runs a query returning 1,000,000 rows with `--max-rows 0`
- **THEN** all rows are fetched and printed with no truncation footer

### Requirement: Result-set handle cleanup

When a result set was delivered via a result-set handle, the client SHALL close the handle after the result has been consumed (including when consumption stops early due to a row limit), so that the server can release the associated resources.

#### Scenario: Handle closed after consumption

- **WHEN** a query result delivered via a result-set handle has been fully read or truncated by a row limit
- **THEN** the client closes the result-set handle on the server

### Requirement: Non-resultset statements remain supported

Statements that do not produce a result set (e.g. `OPEN SCHEMA`, DDL, DML row counts) SHALL continue to execute successfully and present an empty result.

#### Scenario: OPEN SCHEMA

- **WHEN** the user executes `OPEN SCHEMA sample;`
- **THEN** the statement succeeds and no result rows are printed

### Requirement: Truthful retrieval error messages

If the retrieved result data does not match the announced shape, the error message SHALL report the actual expected and received values for the dimension being checked (columns compared with columns, rows compared with rows).

#### Scenario: Shape mismatch reported correctly

- **WHEN** the client detects a mismatch between the announced column count and the received data
- **THEN** the error message reports the expected column count (not the row count) alongside the received value
