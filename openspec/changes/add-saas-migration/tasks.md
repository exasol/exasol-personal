# Tasks

> First increment landed: SaaS API client + state, the full `saas` command group,
> and a working migration engine (schemas, tables with distribution keys, data via
> `EXPORT ... INTO EXA`, views, scripts, validation). Remaining: users/roles/privilege
> replay, broader tests, and the how-to doc. SaaS API endpoint paths are still to be
> confirmed against the live spec (isolated in `internal/saas/client.go`).

## 1. SaaS API client & state (`internal/saas`)
- [x] 1.1 Add a SaaS REST API client with `Authorization: Bearer` auth.
- [x] 1.2 Persist/read `saasToken` and `saasDbPassword` in `secrets.json` (mask in output, preserve `0600`).
- [x] 1.3 Persist/read the resolved `saas` block (account, region, db_uuid, host, port, fingerprint, username) in `deployment.json`.
- [x] 1.4 Implement token validation (`GET /accounts/{accountId}`).
- [x] 1.5 Implement target resolution (`GET /accounts/{accountId}/databases/{dbUuid}`).
- [x] 1.6 Implement allowed-IP read/add (`GET`/`PUT .../allowedIPs`) with idempotency.
- [x] 1.7 Implement egress-IP auto-detection via an external echo service, appending `/32`.

## 2. `saas` command group (`cmd/exasol`)
- [x] 2.1 Add the `saas` parent command and shared token-gating check.
- [x] 2.2 `saas token` — define/validate/store; `--account`, `--region`, `--show`, `--clear`.
- [x] 2.3 `saas login` — interactive flow scaffold; fall back to `saas token` while WIP.
- [x] 2.4 `saas allow-ip` — no-arg egress detection, explicit IP/CIDR, `--db`, `--comment`.
- [x] 2.5 `saas test-connection --db` — ordered, non-destructive checklist; fail-fast exit code.
- [x] 2.6 `saas migration --db` — `--schema`, `--dry-run`, `--objects-only`, `--data-only`.

## 3. Migration engine
- [~] 3.1 Enumerate source objects from catalog views (schemas, tables, columns/dist keys, views, scripts, connections) — users/roles/privileges enumeration still pending; row-count baseline captured at validation time.
- [~] 3.2 Recreate schemas and tables with `DISTRIBUTE BY` on the target — users/roles/connection recreation still pending (reported as manual; see 3.5).
- [x] 3.3 Transfer table data via `EXPORT ... INTO EXA` with `TRUNCATE` into pre-created tables.
- [~] 3.4 Recreate views and scripts after data load — privilege replay still pending.
- [x] 3.5 Surface non-migratable secrets (connection credentials) for manual setting.
- [x] 3.6 Run `test-connection` first and abort migration on failure.
- [x] 3.7 Validate per-table row counts against the source after transfer.

## 4. Documentation & tests
- [ ] 4.1 Add a SaaS migration how-to doc.
- [~] 4.2 Unit tests: token validation, egress detection, target resolution, allowed-IP request shape — gating and allow-ip idempotency tests still pending.
- [~] 4.3 Unit tests: distribution-key preservation, dry-run output (no secrets leaked, no DDL on source) — row-count mismatch path still pending.
