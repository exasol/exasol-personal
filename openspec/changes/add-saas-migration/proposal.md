## Why

Users running a local Exasol deployment have no launcher-driven path to move their schema, data, and database objects into an Exasol SaaS database. Doing it by hand requires composing `EXPORT ... INTO EXA` statements, manually managing the SaaS account token, the IP allowlist, and the target connection details, and replaying object DDL that `EXPORT` does not carry. This change adds first-class launcher commands that drive a one-shot migration from the local deployment to a SaaS database.

## What Changes

- Add a `saas` command group to the launcher for SaaS account access and migration.
- Add `saas token` to define and store a SaaS account access token, validated against the SaaS API before it is persisted.
- Add `saas login` for interactive (browser/device) login that obtains a token *(work in progress; falls back to `saas token`)*.
- Add `saas allow-ip` to add the local deployment's egress IP (or an explicit IP/CIDR) to the SaaS allowed-IP list.
- Add `saas test-connection` to perform a non-destructive, dry-run connectivity check to a target SaaS database.
- Add `saas migration` to run the migration into a target SaaS database identified by its `db_uuid`: replay objects (schemas, table DDL with distribution keys, users/roles, connections, views, scripts, privileges) and transfer table data via `EXPORT ... INTO EXA`.
- Gate `allow-ip`, `test-connection`, and `migration` on a defined token.
- Update documentation to describe the SaaS migration workflow.

## Capabilities

### New Capabilities
- `saas-migration`: how the launcher authenticates to a SaaS account, prepares connectivity, and migrates a local deployment's schema, data, and database objects into a SaaS database.

### Modified Capabilities
<!-- None: no existing permanent spec covers SaaS access or migration. -->

## Impact

- `cmd/exasol`: add the `saas` command group (`token`, `login`, `allow-ip`, `test-connection`, `migration`).
- `internal/saas` (new): SaaS REST API client, token storage/validation, allowlist management, target resolution, egress-IP detection.
- `internal/connect`: reuse SQL execution to issue `EXPORT ... INTO EXA` and object-DDL replay against the local source database.
- `internal/config` / deployment state: persist `saasToken` / `saasDbPassword` in `secrets.json` and the resolved `saas` block in `deployment.json`.
- Documentation: add a SaaS migration how-to.
- Tests: unit tests for token validation/gating, allowlist idempotency, egress-IP detection, target resolution, migration ordering, and dry-run output.
- New external dependency: outbound HTTPS to the SaaS REST API and an egress-IP echo service.
