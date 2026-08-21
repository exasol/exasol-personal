## Context

The launcher delegates SQL execution to `github.com/exasol/exasol-driver-go` through the existing `exasol connect` command. The current dependency is v1.0.17, while upstream v1.1.0 adds the client-side protocol support required by `IMPORT FROM LOCAL PARQUET`. Existing end-to-end tests already exercise local CSV imports and provide the deployment and cleanup fixtures needed for this workflow.

## Goals / Non-Goals

**Goals:**

- Consume the upstream driver capability without reimplementing file transfer logic in the launcher.
- Verify that a Parquet file remains on the client side and is imported through `exasol connect`.
- Make the supported invocation discoverable in the README.

**Non-Goals:**

- Changing Exasol SQL grammar or engine-side Parquet support.
- Supporting cloud/object-storage Parquet scenarios in this change.
- Documenting every Parquet schema or column-mapping limitation.

## Decisions

- Upgrade the direct Go module requirement to v1.1.0 and retain the existing connection abstraction. This keeps the change at the integration boundary; a launcher-specific Parquet implementation would duplicate upstream behavior and create an unwanted product surface.
- Add the validation beside the existing local CSV E2E coverage, using a small checked-in Parquet fixture and a query that proves rows were imported. A unit test cannot exercise the driver protocol or client-file access, while a deployment E2E test validates the complete user path.
- Keep the README example focused on `IMPORT FROM LOCAL PARQUET` through `exasol connect`, and state that this scope does not add engine or cloud-import behavior. The README is the end-user entry point for the existing sample-data workflow.

## Risks / Trade-offs

- [Driver v1.1.0 compatibility] → Run the Go unit suite and the relevant local E2E test; retain the existing driver API integration.
- [Test fixture portability] → Use a minimal Parquet file generated with a stable test-data tool or checked-in binary fixture, and run the test only in deployment environments that support the existing E2E markers.
- [Local deployment availability] → Keep automated coverage aligned with the current local/cloud E2E fixture conventions and report release validation separately if the environment cannot run it.

## Migration Plan

Upgrade the module, run validation, and release normally. Rollback is the standard dependency revert if the upstream driver introduces an incompatibility; no database migration or persisted-state change is required.

## Open Questions

None for the launcher integration. Engine-side limitations remain outside this issue.
