## Why

Local Exasol Personal deployments ship no script language containers (SLCs): the nano
image omits them to stay within its size and download budget. As a result a `PYTHON3`,
`JAVA`, or `R` UDF fails until the user provisions an SLC by hand, even though most
examples online assume those languages exist. Users need a first-class, one-command way
to install an official SLC into a local deployment.

## What Changes

- Add an `exasol slc` command group: `install <alias>`, `list`, `update <alias>`, and `remove <alias>`.
- Resolve the requested alias (e.g. `python3`) against an embedded, pinned catalog of
  official SLCs (`assets/resources/slc-catalog.yaml`) to a concrete container image
  reference.
- On install, mount the official SLC image into the local database at `/exa/slc/<dir>`
  (Podman image mount). The database activates the language through its built-in scan of
  `/exa/slc` on start — no `ALTER SYSTEM`.
- Persist the set of installed SLCs in launcher state and re-apply it on every start,
  because image mounts are container-run arguments and do not persist across recreation.
- Enforce alias uniqueness across the installed set: two SLCs exposing the same
  unversioned alias (e.g. `PYTHON3`) prevent the database from starting, so a conflicting
  install is rejected.
- Scope to local (darwin/arm64) deployments only; other backends report the operation as
  unsupported.
- Depends on a companion change in the embedded runner to accept SLC image mounts on
  `start` and to harden database-container recreation. See `design.md`.

## Capabilities

### New Capabilities

- `local-slc-management`: install, list, update, and remove official script language containers
  in local deployments.

### Modified Capabilities

<!-- None. -->

## Impact

- `cmd/exasol`: new `slc` command group (`install`, `list`, `update`, `remove`).
- `internal/deploy`, `internal/localruntime`: pass installed SLC mounts to the runner
  `start` command; persist installed-SLC state; restart and verify on install/update/remove.
- `assets/resources`: embed and load `slc-catalog.yaml`.
- the embedded runner (external dependency, re-embedded via `tools/localrunner`): new `--slc`
  start flag that mounts each requested image into the database container, plus hardened
  container cleanup. The interface the launcher depends on is described in `design.md`.
- Tests: catalog resolution, alias and collision logic, command behavior, and
  local-runtime integration.
