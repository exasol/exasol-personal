## Context

`internal/launcherpaths` currently resolves one flat root, `<home>/.exasol/personal`,
identically on every platform. Everything launcher-owned nests under it or, in two
cases, doesn't nest under anything launcher-owned at all:

- Managed deployments (`internal/config/deployment_dir.go`) and the runtime-artifact
  cache (`internal/runtimeartifacts/cache.go`) both sit under the flat root.
- The runtime-artifact cache's own config file, `runtime-artifacts.yaml`, is written
  directly into the flat root, i.e. into the deployments root, not the cache it
  configures.
- SQL history (`internal/connect/shell.go`) is written as `exasol_history` directly
  into `os.UserCacheDir()`, with no launcher namespace at all.

This is also the change that puts launcher configuration in the platform's
configuration location for the first time. Nothing in the codebase writes to that
location today.

## Goals / Non-Goals

**Goals:**

- Every launcher-owned path lives under one `exasol/launcher` namespace per platform,
  in the platform-conventional location for its lifecycle (durable data, config, or
  cache).
- Existing users keep their deployments, SQL history, and cache retention setting
  across the upgrade, with no data loss and no silent overwrite on a naming collision.
- The disposable resource cache is treated as disposable: it is deleted, not migrated.
- Each subsystem's settings live in their own file at the configuration location, so a
  later subsystem adds its own file without touching the resource cache's.

**Non-Goals:**

- The broader SPOT-32441 resource-resolution pipeline redesign (unifying HTTP/Git/
  local/container/embedded sources, preset-scoped resources). This change only moves
  and renames the resource cache's on-disk location and config file.
- Renaming the `internal/runtimeartifacts` Go package, its types, or its terminology.
- Moving deployment-specific configuration out of its deployment directory.
- Preserving the legacy resource cache's contents.

## Decisions

1. **Platform-specific roots, resolved per purpose, not per flat root.**

   `internal/launcherpaths` currently exposes a single `RootDirPath`/`DirPath` pair.
   This change replaces it with one function per purpose (deployments root, history
   file, config file, cache root), because the four purposes don't share one base
   directory consistently across platforms. Every launcher-owned path, including
   the default and named deployment directories and the legacy locations, resolves
   there:

   | Purpose | Linux | macOS | Windows |
   | --- | --- | --- | --- |
   | Managed deployments | `~/.exasol/launcher/deployments` | `~/Library/Application Support/exasol/launcher/deployments` | `%LOCALAPPDATA%\exasol\launcher\deployments` |
   | SQL history | `~/.exasol/launcher/history/exasol_history` | `~/Library/Application Support/exasol/launcher/history/exasol_history` | `%LOCALAPPDATA%\exasol\launcher\history\exasol_history` |
   | Resource cache configuration | `${XDG_CONFIG_HOME:-~/.config}/exasol/launcher/resources.yaml` | `~/Library/Application Support/exasol/launcher/resources.yaml` | `%APPDATA%\exasol\launcher\resources.yaml` |
   | Resource cache | `${XDG_CACHE_HOME:-~/.cache}/exasol/launcher/resources` | `~/Library/Caches/exasol/launcher/resources` | `%LOCALAPPDATA%\exasol\launcher\resources` |

   On Linux, deployments and history deliberately do *not* follow `os.UserConfigDir()`
   or an XDG data-home convention: they keep a stable, discoverable dotfile location
   analogous to the current `~/.exasol/personal`, distinct from configuration. On
   macOS, `os.UserConfigDir()` already resolves to `Application Support`, so
   deployments, history, and configuration share that base and differ only by leaf
   name. On Windows, deployments and history use `%LOCALAPPDATA%` for both local and
   cloud deployments: deployments hold machine-bound state (OpenTofu state with
   absolute paths, SSH keys, local database data), and `%APPDATA%` roams with the
   user profile. Configuration stays in `%APPDATA%`. The cache purpose always uses
   `os.UserCacheDir()` (or its Linux XDG equivalent), on every platform.

2. **Migrate deployments and history; delete, don't migrate, the cache.**

   Deployments and history are user-managed or user-generated data, so losing them is
   a real regression. The resource cache is rebuilt transparently on next use, so this
   change deletes the legacy cache directory outright instead of copying it, matching
   the existing precedent of treating this cache as fully disposable (see
   `runtime-artifact-cache`).

3. **Collision handling refuses to guess.**

   If a deployment name exists at both the legacy and new managed-deployments root,
   silently preferring either one risks discarding the other's state. Migration
   keeps both directories unchanged and warns naming both paths, asking the user to
   rename or remove one side. The other legacy items migrate as usual.

4. **Migration runs at every startup, before any operation is recorded in progress,
   without prompting, and warns instead of failing.**

   Migrating a deployment directory while a command may be mid-operation (e.g.
   destroying cloud resources) risks a race between the migration and the command's
   own use of that directory. Startup migration runs before `cmd/exasol` dispatches to
   any command, so a command never observes a half-migrated deployment tree. Another
   launcher process or a running local database can still use a legacy deployment,
   so migration keeps a legacy deployment at its legacy location, with a warning
   naming the command that frees it, while it is locked, while its local database
   runs, or while its running state cannot be determined. Migration runs at every
   startup, so a kept item moves on the first invocation after nothing keeps it.

   Migration does not prompt for confirmation, matching every other automatic
   migration already in this codebase (`internal/localruntime/mac_data_migration.go`
   migrates a VM's Nano data with no user prompt). It does, however, tell the user
   what happened: a clear human-facing message on standard error naming what moved or
   was deleted and where, emitted before the requested command's own output.

   If migration cannot move or delete an item (an I/O error, a collision, a
   deployment in use), the CLI warns naming the item and the cause, continues with
   the remaining items, and dispatches the requested command. The next invocation
   retries the items still at their legacy location.

5. **Each item moves with one atomic rename.**

   A rename leaves an item complete at exactly one location, so an interrupted run
   resumes by migrating whatever is still at its legacy location. Legacy and new
   locations sit below the same home directory on every platform, so a rename covers
   the default layouts. A layout that puts them on different filesystems (e.g. a
   separately mounted `XDG_CACHE_HOME`) gets a warning naming both paths, and the
   user moves that item by hand.

6. **One settings file per subsystem, each owned by the subsystem itself.**

   `resources.yaml` holds only the resource cache's settings, so
   `internal/runtimeartifacts` reads and writes it directly, depending on
   `internal/launcherpaths` for path resolution alone. This keeps the ownership
   boundary the package already needs: `internal/runtimeartifacts` must not depend on
   `internal/config` (the `launcherpaths` package doc calls this out explicitly, to
   avoid an import cycle risk if `internal/presets` ever depends on
   `internal/runtimeartifacts`), and a file read only by its owner never has to. A
   later subsystem that needs persisted settings adds its own file at the same
   configuration location.

7. **The cache config file keeps its own identity.**

   `runtime-artifacts.yaml`'s only field, `retention_days`, stays as it is in
   `resources.yaml` at the configuration location, its name matching the `resources`
   cache directory leaf. Launcher settings files use YAML, as the rest of the
   repository does. On first run after upgrade, if the legacy file exists, its value
   moves to the new location with the rename from Decision 5, as part of the same
   migration that relocates deployments and history. The cache only creates
   `resources.yaml` on first use, so the new file does not exist yet at startup.

## Risks / Trade-offs

- Running migration at every startup adds a few file stats to every invocation once
  the legacy locations are gone. While a legacy deployment stays kept, every
  invocation checks its lock and running state again and repeats its warning.
- Several invocations started together migrate the same legacy paths. Each treats an
  item another invocation already moved as migrated, so all of them proceed without
  a lock that a killed process could leave behind.
- A command that runs while a legacy deployment is kept resolves only the new
  locations, so the user selects the kept deployment with its legacy
  `--deployment-dir` path, as the warning shows.
- A deployment an earlier launcher version creates at the legacy location after the
  upgrade migrates on the next invocation, or is kept with a collision warning when
  the name exists at the new location.
- Migrating without a confirmation prompt means a user's deployments, history file,
  and cache config move (and the legacy cache is deleted) without them opting in at
  that exact moment. This matches existing precedent in this codebase (no other
  migration prompts either) and is mitigated, not eliminated, by the informational
  message: a user who doesn't read CLI output can still be surprised that legacy
  paths are gone.
- Warning instead of failing lets a command run while some items are still at their
  legacy location. Each item stays complete at one location, and the warning repeats
  on every invocation until the user resolves it.
- A layout with legacy and new locations on different filesystems warns on every
  invocation until the user moves the reported item by hand. Such layouts are
  uncommon, and the rename keeps every item complete at one location.

## Migration Plan

Migration is not a separate rollout step: it runs as part of every CLI invocation's
startup sequence (see Decision 4). Shipping the new launcher version is the only
rollout action; the first invocation of that version against an existing
installation is what triggers it.

Order of operations within one CLI startup:

1. Move the cache configuration to `resources.yaml`.
2. Relocate the resource cache root and delete the legacy cache directory (no cache
   content is migrated).
3. Migrate the SQL history file to its new location.
4. Migrate each deployment directory that has no name collision, is not locked, and
   is known not to run a local database.
5. Report what moved or was deleted in a human-facing message on standard error,
   and warn about each item kept at its legacy location.
6. Remove the legacy directories that are now empty and dispatch to the requested
   command.

A failure at any step produces a warning, and the next invocation retries; steps that
already succeeded are safe to repeat because there is nothing left at the legacy
location to move.

**Rollback: not supported, by design.** Deployments, history, and the cache config are
moved rather than copied: once migration completes, the legacy locations no longer
hold that data, so downgrading to a launcher version that only reads legacy paths will
not find it there. This is accepted deliberately rather than mitigated: a durable
backup would add ongoing disk usage and cleanup complexity in service of a downgrade
path this change does not intend to support.
