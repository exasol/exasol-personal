## 1. Dependency and specification

- [x] 1.1 Upgrade `github.com/exasol/exasol-driver-go` to v1.1.0 and refresh module checksums.
- [x] 1.2 Add the local Parquet import capability and SQL-client behavior to the permanent OpenSpec specifications when archiving.

## 2. Validation

- [x] 2.1 Add a minimal Parquet fixture and an E2E test covering successful client-local import through `exasol connect`.
- [x] 2.2 Add a missing-file or failure-path assertion for local Parquet imports.

## 3. User guidance

- [x] 3.1 Update the README with a supported `IMPORT FROM LOCAL PARQUET` example and concise scope/non-goals guidance.
- [x] 3.2 Run formatting, unit/integration validation, and the relevant E2E test where the environment supports it.
