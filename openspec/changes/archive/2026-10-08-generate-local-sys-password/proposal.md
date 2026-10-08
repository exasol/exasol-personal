## Why

Every local deployment initializes the `sys` administrator with the publicly known password `exasol`, which creates avoidable exposure when a local database is reachable beyond its intended boundary. Product Security identified this as a known, preventable weakness.

## What Changes

- Initialize `sys` in each fresh local deployment with a launcher-generated password, supplied to Nano once through its `sys_password_file` interface.
- Store the password in `secrets.json` before first initialization, keep it across start, stop, retry, and recovery, and verify after first initialization that the database accepts it.
- Keep the stored credential of existing local deployments.
- Remove temporary bootstrap material after first initialization succeeds or fails, and stop logging the database password in connection debug logs.
- Run the first-initialization container without automatic restart, because Nano rejects initial-password options on an initialized runtime.
- Update user documentation that states new local deployments use `sys` / `exasol`.

## Capabilities

### New Capabilities

- `credential-confidentiality`: Describe database connections in launcher logs without the connection password on every deployment type.

### Modified Capabilities

- `exasol-local-deployment`: Replace the fixed local credential with a generated, persisted, and verified one, and define first-initialization, retry, existing-deployment, and bootstrap-material behavior.

## Impact

The change affects the shared Podman installation policy, local runtime paths, the local deploy workflow and artifact writing, secrets persistence, connection logging, local lifecycle tests, user documentation, and release notes. Cloud password generation is unchanged.
