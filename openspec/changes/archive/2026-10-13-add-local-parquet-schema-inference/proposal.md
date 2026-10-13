## Why

Evaluators usually start with their own local Parquet files. Today `exasol connect` imports such a
file only into a table the user created beforehand with hand-written column types, while the same
file in object storage loads into a new table with one statement and inferred types. That friction
sits at the first data load, where evaluators decide whether to continue (SPOT-33139).

## What Changes

- `exasol connect` recognizes `CREATE [OR REPLACE] TABLE <schema>.<table> AS (IMPORT FROM LOCAL
  PARQUET FILE '<path>')` and creates the table from the file's Parquet metadata before importing
  its rows, using exasol-driver-go 1.2.0.
- Column names keep the Parquet names, matching the database's inference for object storage.
- A statement that combines table creation with a local Parquet import in any other shape fails
  with an error naming the supported form.
- The user documentation describes the statement and its limitations.

## Capabilities

### Modified Capabilities

- `local-parquet-import`: adds creating a new table with an inferred schema from a local Parquet
  file.

## Impact

- `exasol connect` statement execution and the exasol-driver-go dependency.
- No change to other statements, including `IMPORT INTO <table> FROM LOCAL PARQUET`.
