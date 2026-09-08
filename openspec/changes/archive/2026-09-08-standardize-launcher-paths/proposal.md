## Why

Launcher-managed files currently share one flat, Linux-flavored root
(`~/.exasol/personal`) on every platform, and not everything even lives there: SQL
history is written straight into the OS cache directory with no launcher-owned
namespace at all, and the runtime-artifact cache's own configuration file sits in the
deployments root instead of the platform's configuration location. Users on macOS and
Windows get a Linux dotfile layout instead of their platform's conventional locations,
and nothing distinguishes "data I manage on purpose" (deployments) from "state I can
always reconstruct" (cache) when deciding what to back up, sync, or delete.

## What Changes

- Introduce one `exasol/launcher` namespace per platform, split by lifecycle:
  managed deployments and SQL history under a durable, platform-conventional root;
  configuration under the platform's config directory (`XDG_CONFIG_HOME` on Linux,
  `Application Support` on macOS, `%APPDATA%` on Windows); the resource cache under
  the platform's cache directory (`XDG_CACHE_HOME` on Linux, `Caches` on macOS,
  `%LOCALAPPDATA%` on Windows).
- Move the runtime-artifact cache's configuration to the new configuration location as
  `resources.yaml`, a standalone file owned by the cache. It is currently
  `runtime-artifacts.yaml` in the deployments root; this change renames it and moves it
  to the platform's configuration location.
- Rename the on-disk resource-cache directory leaf from `runtime-artifacts` to
  `resources`, matching its config file name. This is a paths/naming change only:
  the `internal/runtimeartifacts` Go package, its types, and its terminology are left
  alone; the broader resource-resolution pipeline redesign remains separate, ongoing
  work (SPOT-32441).
- Migrate existing deployments, configuration (the cache retention setting), and SQL
  history from their legacy locations to the new ones. The legacy resource cache is
  deleted rather than migrated, since it is disposable and rebuilt on demand. Legacy
  directories left empty are removed.
- Detect a naming collision between a legacy and new-location deployment directory,
  keep both unchanged, and warn identifying both paths.
- Run this migration early at every CLI startup and before any command records an
  operation in progress against a deployment, until every legacy item is moved, so
  a kept or interrupted item moves on a later invocation.
- Migrate without prompting for confirmation, matching every other automatic
  migration already in this codebase, but always tell the user what happened: a clear
  message naming what moved or was deleted and where.
- Keep a legacy deployment in use, or one whose running state cannot be determined,
  at its legacy location and warn with the command that frees it.
- Warn about an item migration cannot move and still run the requested command.
- Move each item with a single atomic rename, so an interrupted migration leaves
  every item complete at either its legacy or its new location.
- Update command help and user documentation to describe the new locations.

## Capabilities

### New Capabilities

- `sql-history-storage`: define where the interactive shell's SQL history persists and
  how an existing legacy history file is carried over.
- `launcher-path-migration`: the cross-cutting migration mechanics shared by every
  migrated location (deployments, history, cache config), covering user notification,
  warning on failure, and atomic moves.

### Modified Capabilities

- `deployment-directory-resolution`: managed deployments (and the default/named
  deployment directories it resolves) move from the flat legacy root to the new
  platform-specific managed-deployments root, with startup migration and collision
  detection from the legacy location.
- `runtime-artifact-cache`: the cache root moves to the new platform-specific cache
  location under the `resources` leaf name, and its retention configuration moves to
  `resources.yaml` at the new configuration location.
- `deployment-directory-listing`: the listing enumerates the managed-deployments root
  defined by `deployment-directory-resolution` instead of restating a path of its own.

## Impact

- `internal/launcherpaths`: resolves every launcher-owned path per purpose and per
  platform instead of one flat root, including the legacy locations.
- `internal/config/deployment_dir.go`: managed deployment paths move to
  `internal/launcherpaths`.
- `internal/runtimeartifacts/cache.go`: cache root moves to the new cache location
  under the `resources` leaf name; its config file moves to the new configuration
  location as `resources.yaml`.
- `internal/connect/shell.go`: SQL history moves from an unnamespaced file directly in
  the OS cache directory to the new history location.
- `cmd/exasol`: wire the startup migration before command dispatch,
  moving each item with one atomic rename, detecting deployment name collisions,
  and reporting what moved or was deleted.
- User docs: `user-docs/content/manage-deployments.md` and any other prose or example
  output naming `~/.exasol/personal`.
- OpenSpec specs: `deployment-directory-resolution`, `deployment-directory-listing`,
  and `runtime-artifact-cache` (modified), new `sql-history-storage` and
  `launcher-path-migration` capabilities.
