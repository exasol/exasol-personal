# Version checking

This document describes the **update-checking** features in Exasol Personal.

There are three related mechanisms:

- **Launcher version check:** the Exasol Personal launcher checks whether a newer launcher release is available.
- **Database version check:** during installation, the launcher can enable the Exasol database’s own daily version check.
- **Host-side launcher version check:** a temporary fallback on the deployed host system can periodically check for newer launcher releases without requiring CLI usage.

## Launcher version check (launcher updates)

### Goals and behavior

The version check is intentionally **best-effort**:

- It should **never block** a command from running.
- It should be **rate limited** so normal command usage does not generate excessive requests.
- It should provide an **actionable hint** (and point users to `exasol version --latest` for details).

### When the launcher checks

- Most `exasol` commands (everything except `exasol version`) attempt a version check during command startup.
- The check is performed only when the launcher has access to a **deployment directory**, because the deployment’s persistent launcher state is used for rate limiting. For background on launcher state, see [Deployment state & locking](launcher_state.md).
- `init` and `install` are special: they are frequently run *before* a deployment directory exists. They therefore perform the check directly as part of their workflow.

#### Opting out

Users can disable automatic launcher update checks during initialization (for example, in CI or offline environments) using `--no-launcher-version-check`.

### Rate limiting and failure semantics

The launcher performs at most one check per **deployment directory** within a 24-hour window.

To avoid repeated requests in error cases, an attempt is treated as a “check” even when the API call fails or times out; another attempt will not be made for the same deployment directory for 24 hours.

### API endpoint

By default, the launcher uses:

- `https://metrics.exasol.com/v1/version-check`

The request includes basic platform and version information (category, operating system, CPU architecture, and current version) so the service can return the latest compatible release artifact.

The endpoint can be overridden via the environment variable `EXASOL_VERSION_CHECK_URL` (primarily used for tests).

The launcher sends the following query parameters:

- `category`
- `operatingSystem`
- `architecture`
- `version`
- `identity`

The response contains a `latestVersion` object with metadata for the newest available release on that platform (version, download URL, checksums, and platform).

## Database version check (daily DB update awareness)

Some Exasol database releases include an internal, non-disruptive **daily version check** (performed by the database/cluster services).

When that capability is available, Exasol Personal configures it during installation so new deployments gain update awareness “out of the box”, while still allowing operators to opt out.

### Default behavior and CLI opt-out

- **Default (new deployments):** enabled during installation of Exasol on the host systems unless the user opts out.
- **Existing deployments:** are not changed implicitly by upgrading the launcher; the setting remains whatever was recorded for that deployment.

Opt-out is exposed as an installation-time CLI setting (for example a boolean flag/variable such as `--no-db-version-check`). The help text documents:

- that the default is enabled for new Exasol Personal installs
- that the check is best-effort and non-blocking
- how to disable it for offline/controlled environments

### Persistence and idempotence

The launcher persists the user’s choice in the deployment directory’s configuration (the same place other installation/deployment settings are recorded).

This ensures follow-up lifecycle actions that reuse the install plan (for example reinstall, scaling, or node replacement) keep behaving consistently without requiring users to repeat the opt-out/opt-in setting.

Repeated applies are idempotent:

- When enabled, the resulting database configuration has `versionUpdateCheck: true`.
- When disabled, the database behaves as default-off.

### Configuration passed during installation

At installation time, the launcher translates the CLI choice into installation preset variables and materializes the resolved values on the host.

Conceptually, installation scripts read installation-owned configuration (commonly from `/etc/exasol_launcher/installation.json`) and configure these c4 “host play” variables:

- `CCC_PLAY_VERSION_UPDATE_CHECK` (enable/disable)
- `CCC_PLAY_VERSION_UPDATE_CHECK_ENDPOINT` (endpoint override; optional)
- `CCC_PLAY_CLUSTER_IDENTITY` (stable identity used for API requests)

If the DB version check is enabled (that is, `no_db_version_check` is false), `CCC_PLAY_VERSION_UPDATE_CHECK` is set accordingly; otherwise it is left absent/false.

### Cluster identity

For Exasol Personal deployments, installation sets a predictable `CCC_PLAY_CLUSTER_IDENTITY` so version-check API requests can be attributed to a stable (but non-personal) deployment identity.

The identity format is:

`exasol-personal;<deployment-id>;<infra-preset-name>;<install-preset-name>`

## Host-side version check fallback (temporary)

In addition to launcher-initiated checks, the deployed host system performs periodic version checks against the same version-check API endpoint.

This mechanism exists as a backup for the database version check. It is intended for cases where the database-side daily check is unavailable or not used, so long-running deployments can still report update activity and receive launcher update information even when operators do not invoke the CLI regularly.

Because it is only a fallback, it is expected to be temporary and may be removed once the database version check is the sole mechanism needed for scheduled update awareness.

### Scope and activation

- The scheduled check runs on the **access node** only, so each deployment performs the check from exactly one host.
- It becomes active only after the database is reported as ready.
- It is enabled only when the database version check is enabled for the deployment.
- It is installed as part of the installation preset and runs on a host-local schedule managed by the operating system, with a `systemd` service and timer being the intended implementation vehicle.

### Behavior and failure semantics

The host-side check is intentionally **best-effort**:

- It must not affect database availability or block normal deployment operations.
- It uses the same endpoint base URL as launcher version checks (including launcher-side endpoint overrides).
- It uses a database-compatible request profile (`category=Exasol 8`, `operatingSystem=Linux`, `architecture=x86_64`) and sends the configured Exasol version from installation variables.
- It is rate limited so failures, including timeouts and temporary connectivity problems, do not create tight retry loops.
- Failed attempts are still recorded for local bookkeeping so the next retry is deferred by the normal schedule rather than happening immediately.

### Deployment identity

The scheduled host-side check uses a stable deployment identity so the service can attribute requests to a deployment over time without relying on CLI activity.

For Exasol Personal deployments, that identity is privacy-conscious: it is stable enough for per-deployment behavior and metrics, but it does not expose sensitive operator or environment data.

The configured identity is the same `CCC_PLAY_CLUSTER_IDENTITY` value used for the database version check.
