# Design: SaaS migration

## Context

The local deployment (`backend: local`, single-node macOS VM, e.g. `127.0.0.1:58653`, admin `sys`) holds schemas, tables (with `DISTRIBUTE BY` distribution keys), and objects (views, scripts/UDFs, connections, users/roles, privileges). The goal is a **one-shot** migration of schema + data + objects into an Exasol SaaS database. Incremental/continuous sync, client/application reconnection, and production cutover orchestration are out of scope.

## Migration method

Two mechanisms, run in dependency order:

- **Table data** → [`EXPORT ... INTO EXA`](https://docs.exasol.com/db/latest/sql/export.htm). The statement runs **on the local (source) DB** and streams rows directly into the target Exasol DB over an `EXA` connection — no intermediate file staging. The source is read-only throughout.
- **Everything else** → extract DDL from source system tables and **replay** it on the target. `EXPORT INTO EXA` moves only rows (and optionally a bare `CREATE TABLE` via `CREATED BY`); it does not carry distribution keys, views, scripts, connections, users, or grants.

### `EXPORT ... INTO EXA` reference

```sql
-- Connection FROM local source DB TO SaaS target. Encryption is mandatory for SaaS;
-- the SaaS endpoint presents a publicly trusted certificate, so no fingerprint is pinned.
-- The SaaS token is the credential (used as the password); only the db user is supplied.
CREATE OR REPLACE CONNECTION saas_target
    TO '<host>:8563;Encryption=Y'
    USER '<saas_db_user>' IDENTIFIED BY '<saas_token>';

-- Target tables are pre-created with full DDL (incl. DISTRIBUTE BY), so use TRUNCATE
-- for a clean reload. TRUNCATE cannot be combined with REPLACE or CREATED BY.
EXPORT SAMPLE.PRODUCTS INTO EXA AT 'saas_target' TABLE SAMPLE.PRODUCTS TRUNCATE;
```

Target option matrix (`EXPORT ... INTO EXA ... TABLE <t> [option]`):

| Option | Effect | Use |
|---|---|---|
| *(none)* | append into existing target | additive loads |
| `TRUNCATE` | empty target first, then load | **default** — clean reload |
| `REPLACE` | drop & recreate target first | discard target DDL |
| `CREATED BY 'CREATE TABLE ...'` | create from inline DDL, then load | when not pre-creating (loses `DISTRIBUTE BY` unless included) |

Required privileges: source user needs `EXPORT` + read on source tables + `USE ANY CONNECTION` (or the EXA connection `GRANT`ed). Target user needs `CREATE SCHEMA`/`CREATE TABLE`/`INSERT` (+ `CREATE VIEW`/`CREATE SCRIPT`/`CREATE USER`/`GRANT` for objects).

### Migration order (respects dependencies)

```
1. Users & roles      (target)   independent
2. Connections        (target)   secrets re-entered manually (source secrets not readable)
3. Schemas            (target)
4. Tables (full DDL,  (target)   includes DISTRIBUTE BY — pre-created before load
   incl. distribution)
5. Table DATA         EXPORT INTO EXA   source → target
6. Views              (target)   depend on tables
7. Scripts / UDFs     (target)
8. Privileges         (target)   depend on objects + users existing
9. Validation                    row counts / aggregate checks
```

### Source object enumeration

Object DDL is derived from source catalog views: `EXA_ALL_SCHEMAS`, `EXA_ALL_TABLES` (+ row counts for validation), `EXA_ALL_COLUMNS` (`column_is_distribution_key`), `EXA_ALL_VIEWS` (`view_text`), `EXA_ALL_SCRIPTS` (`script_text`), `EXA_DBA_CONNECTIONS` (names/strings only — secrets not exposed), `EXA_DBA_USERS`/`EXA_DBA_ROLES`, and `EXA_DBA_SYS_PRIVS`/`EXA_DBA_OBJ_PRIVS`/`EXA_DBA_ROLE_PRIVS`. Passwords and connection secrets cannot be extracted and are set/re-entered on the target.

## CLI command group

| Command | Purpose | Token required |
|---|---|---|
| `exasol saas token <PAT>` | define/store the SaaS account token | — (defines it) |
| `exasol saas login` | interactive browser login *(WIP)* | — (obtains it) |
| `exasol saas allow-ip [IP\|CIDR]` | add an IP to the SaaS allowed-IP list | yes |
| `exasol saas test-connection --db <db_uuid>` | dry-run connectivity check | yes |
| `exasol saas migration --db <db_uuid>` | run the migration into the target DB | yes |

Token gating: `allow-ip`, `test-connection`, `migration` exit non-zero when no token is defined, with `error: no SaaS token defined — set the EXASOL_SAAS_TOKEN environment variable`. The token may come from the `EXASOL_SAAS_TOKEN` env var (preferred) or from `saas token`/`saas login`.

The SaaS token is also the **database connection credential**: the launcher connects to the SaaS database via the driver's OpenID **access-token login** (the `loginToken` protocol command), not username/password. No database user or password is needed for the connection. SaaS clusters present an internal certificate, so server-certificate verification is skipped (TLS encryption stays on).

- `saas token`: `--account <id>`, `--region <region>`, `--show` (masked), `--clear`. Validates against the API and persists only on success.
- `saas allow-ip`: account-level; no-arg auto-detects the egress IP and adds `/32`; accepts explicit IP/CIDR; `--name`; idempotent.
- `saas test-connection --db <db_uuid> --db-user <user>`: non-destructive checklist (token → DB status → allowlist → reachability → auth → `SELECT 1`); fail-fast non-zero exit.
- `saas migration --db <db_uuid> --target-user <user>`: `--schema <name>` (default all non-system), `--dry-run` (print planned DDL + `EXPORT`), `--objects-only` / `--data-only`. Runs `test-connection` first and aborts on failure.

## State & API contracts

### `deployments/<name>/secrets.json` (existing file; new keys, `0600` preserved; token masked in all output)

```json
{ "dbPassword": "…", "saasToken": "exa_pat_…" }
```

### `deployments/<name>/deployment.json` (non-secret resolved SaaS context, cached after first lookup)

```json
{
  "saas": {
    "accountId": "ORG-abc123", "region": "eu-central-1",
    "dbUuid": "a1b2c3d4-e5f6-7890-abcd-ef0123456789",
    "host": "a1b2c3d4.eu-central-1.exasol.com", "port": 8563,
    "username": "saas_migrator"
  }
}
```

### SaaS REST API (auth `Authorization: Bearer <saasToken>`)

> Endpoint paths confirmed against the live SaaS OpenAPI (`https://cloud.exasol.com/openapi.json`). The base URL defaults to `https://cloud.exasol.com/api/v1` and is overridable for testing via the `EXASOL_SAAS_API_URL` environment variable. The allowed-IP list is **account-level** (not per database). The API has no dedicated token/account endpoint, so token validation lists databases.

| Command | Method · path | Notes |
|---|---|---|
| `saas token` (validate) | `GET /accounts/{accountId}/databases` | 2xx JSON ⇒ token + account valid |
| `saas allow-ip` (detect egress) | `GET /internal/my_ip` | caller's public egress IP (plain or `{ ip }`) |
| `saas allow-ip` (read) | `GET /accounts/{accountId}/security/allowlist_ip` | `[{ id, name, cidrIp }]` — idempotency |
| `saas allow-ip` (add) | `POST /accounts/{accountId}/security/allowlist_ip` | body `{ name, cidrIp }` |
| `test-connection`/`migration` (resolve db) | `GET /accounts/{accountId}/databases/{dbUuid}` | `{ id, status }` (connection is resolved separately) |
| resolve connection (clusters) | `GET /accounts/{accountId}/databases/{dbUuid}/clusters` | `[{ id, name, mainCluster }]` — pick the main cluster |
| resolve connection (connect) | `GET /accounts/{accountId}/databases/{dbUuid}/clusters/{clusterId}/connect` | `{ dns, port }` → connection host/port |

> A non-JSON 2xx response (e.g. the SaaS SPA's HTML) is treated as an error with a clear message — it signals a wrong base URL, account id, or path rather than a valid result.

Egress-IP auto-detection calls the authenticated SaaS `GET /internal/my_ip` endpoint (so the IP matches what SaaS sees) and appends `/32`. SaaS endpoints present a publicly trusted TLS certificate, so no fingerprint is pinned.

## Validation

For each migrated table, compare source vs. target row counts against the baseline captured during enumeration, plus an optional aggregate check (e.g. `SELECT SUM(PRODUCT_ID), SUM(PRICE_USD)`). Verify distribution keys on the target via `EXA_ALL_COLUMNS.column_is_distribution_key`, and confirm views/scripts/users/roles/connections/grants exist.

## Risks

| Risk | Mitigation |
|---|---|
| SaaS rejects connection (IP not allowlisted) | `saas allow-ip` before migration; `test-connection` reports it |
| TLS handshake failure | require `Encryption=Y`; SaaS presents a publicly trusted certificate (no fingerprint pinned) |
| Distribution key lost | pre-create tables with `DISTRIBUTE BY`; do not rely on `CREATED BY` |
| Connection/user secrets not migratable | re-enter connection secrets; set new user passwords on target |
| Large tables / timeouts | multi-host SaaS connection string for parallel export; tune `HostTimeOut`/`LoginTimeout` |

## Rollback

The source local DB is untouched (`EXPORT` is read-only on source). Roll back by discarding the SaaS database or `DROP SCHEMA … CASCADE` on the target, then re-run. No source-side recovery required.
