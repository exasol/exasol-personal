## Why

Exasol Personal already exposes a bundled SQL client, but local Parquet imports fail because the launcher still uses an older Go driver. The Go driver v1.1.0 now provides the required client-side Parquet support, so Exasol Personal can make a common local data workflow reliable without adding a launcher-specific workaround.

## What Changes

- Upgrade the bundled Go driver to v1.1.0.
- Verify `IMPORT FROM LOCAL PARQUET` through `exasol connect` against a local deployment.
- Document the supported local Parquet import path and the limits of this scope.

## Capabilities

### New Capabilities

- `local-parquet-import`: Import Parquet files from the client filesystem through the bundled SQL client.

### Modified Capabilities

- `connect-sql-input`: Extend the SQL client contract to support local Parquet import statements.

## Impact

The change affects the Go module dependency, end-to-end test fixtures and deployment tests, and end-user loading documentation. It relies on the upstream `github.com/exasol/exasol-driver-go` v1.1.0 implementation and does not change SQL grammar or engine-side Parquet behavior.
