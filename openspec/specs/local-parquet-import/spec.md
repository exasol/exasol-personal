# local-parquet-import Specification

## Purpose
Define the expected behavior and guarantees for importing client-local Parquet files via `exasol connect`.
## Requirements
### Requirement: Import a client-local Parquet file

The system SHALL allow `exasol connect` to execute `IMPORT FROM LOCAL PARQUET` statements that read a Parquet file from the client filesystem and load its rows into the connected Exasol database.

#### Scenario: Import a local Parquet file through the SQL client

- **WHEN** a running deployment is connected with `exasol connect` and given a valid `IMPORT FROM LOCAL PARQUET` statement referencing a client-local file
- **THEN** the command completes successfully
- **AND** the imported table contains the rows from that file

#### Scenario: Missing local Parquet file

- **WHEN** a local Parquet import references a file that does not exist on the client filesystem
- **THEN** the command exits unsuccessfully with an error
- **AND** it does not report a successful import
