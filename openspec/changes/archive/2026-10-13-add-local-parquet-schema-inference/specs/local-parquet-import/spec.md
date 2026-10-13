## ADDED Requirements

### Requirement: Create a table from a local Parquet file with an inferred schema

`exasol connect` SHALL execute `CREATE [OR REPLACE] TABLE <schema>.<table> AS (IMPORT FROM LOCAL
PARQUET FILE '<path>')` by creating the table with columns derived from the client-local Parquet
file's metadata and importing the file's rows into it. Column names SHALL be the Parquet column
names. Unquoted schema and table names SHALL be upper-cased.

#### Scenario: Create a new table from a local Parquet file

- **WHEN** the statement names a schema that exists, a table that does not exist, and a readable local Parquet file
- **THEN** the command succeeds
- **AND** the table exists with one column per Parquet column, named as in the file
- **AND** the table contains the rows from that file

#### Scenario: Replace an existing table

- **WHEN** the statement uses `CREATE OR REPLACE TABLE` and the table already exists
- **THEN** the command succeeds
- **AND** the table has the file's columns and contains only the rows from that file

#### Scenario: Table already exists without OR REPLACE

- **WHEN** the statement uses `CREATE TABLE` and the table already exists
- **THEN** the command exits unsuccessfully with an error stating that the table exists
- **AND** the existing table keeps its columns and rows

#### Scenario: Missing local Parquet file

- **WHEN** the statement references a file that does not exist on the client filesystem
- **THEN** the command exits unsuccessfully with an error naming the file
- **AND** no table is created and an existing table is left unchanged

#### Scenario: Import fails

- **WHEN** the file cannot be imported, for example because it is not a valid Parquet file
- **THEN** the command exits unsuccessfully with an error
- **AND** no table is created and an existing table keeps its columns and rows

#### Scenario: Unsupported characters in the file path

- **WHEN** the file path contains characters other than ASCII letters, digits, spaces and `: < > / . _ - ~`
- **THEN** the command exits unsuccessfully with an error naming the supported characters
- **AND** no table is created and an existing table is left unchanged

### Requirement: Reject unsupported table creation from local Parquet files

`exasol connect` SHALL reject a statement that starts with `CREATE [OR REPLACE] TABLE` and contains
a local Parquet import in any shape other than the supported one, with an error that names the
supported form, before sending the statement to the database. The check SHALL consider only
keywords outside comments and string literals.

#### Scenario: Local Parquet import inside a subselect

- **WHEN** the user runs `CREATE TABLE s.t AS SELECT * FROM (IMPORT FROM LOCAL PARQUET FILE '/data/f.parquet')`
- **THEN** the command exits unsuccessfully
- **AND** the error names `CREATE [OR REPLACE] TABLE <schema>.<table> AS (IMPORT FROM LOCAL PARQUET FILE '<path>')`

#### Scenario: Keywords inside a literal

- **WHEN** a `CREATE TABLE ... AS SELECT` statement contains `LOCAL PARQUET` only inside a string literal or comment
- **THEN** the statement is executed by the database as before
