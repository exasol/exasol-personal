# connect-sql-input Specification

## ADDED Requirements

### Requirement: Execute local Parquet import statements

The SQL input accepted by `exasol connect` SHALL support the complete `IMPORT FROM LOCAL PARQUET` statement, including a client-local file path, without requiring a separate launcher command or file conversion step.

#### Scenario: Non-interactive local Parquet import

- **WHEN** the user supplies a valid local Parquet import through `exasol connect -c` or `exasol connect -f`
- **THEN** the statement is sent to the database through the Go driver
- **AND** the command exits successfully after the import completes
