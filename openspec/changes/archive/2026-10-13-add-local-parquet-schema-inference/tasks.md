## 1. Create tables from local Parquet files

- [x] 1.1 Upgrade exasol-driver-go to 1.2.0.
- [x] 1.2 Add a token-based recognizer for the supported statement and for unsupported table-creation shapes containing a local Parquet import.
- [x] 1.3 Route recognized statements through `ImportParquetWithInferredSchema` on the user's session, importing into a staging table that replaces or becomes the target only after the import succeeds.
- [x] 1.4 Add unit tests for the recognizer and the execution path, and live tests for creating, replacing, existing-table rejection, failed imports, unsupported paths, missing files, and unsupported shapes.
- [x] 1.5 Describe the statement and its limitations in the user documentation and add a `CHANGELOG.md` entry.
