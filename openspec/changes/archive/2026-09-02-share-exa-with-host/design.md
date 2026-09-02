## Context

Linux and Windows local deployments already bind a directory below the
deployment runtime into Nano at `/exa`. macOS instead binds `/var/lib/exa`
from the VM's ext4 data disk. The VM also exposes a launcher-managed host
directory at `/mnt/host` through VirtioFS.

The Nano image documents one persistent mount at `/exa` and supports a writable
absolute host directory as its source. The full tree includes database storage,
configuration, logs, SLCs, JDBC drivers, and BucketFS content.

The macOS VM data disk mounts at `/var`, so it also holds Podman's image store
and other guest runtime state. Moving Nano data does not remove the need for
that disk. Existing macOS deployments need both the old disk and the host share
available during their first start with the new layout.

The spike found no material database-workload regression on macOS. Synthetic
VirtioFS writes were about 31% to 33% slower than ext4, which is accepted in
exchange for host access and a self-contained deployment directory. Windows
already uses Podman-machine filesystem passthrough but still needs explicit
lifecycle and data-visibility coverage.

## Goals / Non-Goals

**Goals:**

- Keep the complete Nano `/exa` tree in host-visible deployment storage on
  Linux, macOS, and Windows.
- Preserve existing macOS databases while moving them from the guest disk.
- Make migration atomic, conflict-safe, interruption-safe, and retryable.
- Preserve sparse database files and relevant filesystem metadata.
- Keep the macOS VM data disk for Podman and guest runtime state.
- Make host visibility and persistence part of macOS and Windows lifecycle
  verification.

**Non-Goals:**

- Split Nano-managed subdirectories into separate mounts.
- Add user-configurable shared-directory settings.
- Eliminate the macOS VM data disk or move Podman's image store to the host.
- Guarantee identical filesystem performance across platforms.
- Migrate arbitrary user-created data when both source and destination contain
  unrelated Nano runtimes.

## Decisions

1. Expose one stable host path as the complete Nano runtime root.

   Linux and Windows continue using the runtime's host-side `exa` directory.
   On macOS, `local/runtime/exa` is a launcher-managed relative symlink to the
   `exa` directory below the runner's private host share. The VM uses the
   corresponding `/mnt/host` path. This keeps the user-facing host layout the
   same on every platform without mixing runner coordination files into Nano's
   `/exa` tree.

2. Keep platform-specific path selection outside installation policy.

   The local runtimes select the host and execution-environment paths. The
   shared Podman installation continues to receive one data directory and bind
   it at `/exa`. This keeps Nano lifecycle behavior common while leaving
   VirtioFS and Windows path translation in their platform boundaries.

3. Migrate macOS data before the shared installation starts Nano.

   After the runner starts with the existing data disk attached, the macOS
   runtime checks the host layout marker and the legacy `/var/lib/exa` source.
   A populated legacy source and an empty host destination trigger migration.
   Nano remains stopped throughout the copy. Windows and Linux do not run this
   migration because their managed data is already host-side.

   Reusing only the current overlay migration was considered. It is rejected
   because that migration treats every bind-mounted `/exa` as persistent and
   skips it. The old macOS container has a valid bind mount whose source is the
   guest disk, which is exactly the source that must move.

4. Copy into a host-side staging directory and publish it atomically.

   The fixed staging path is private launcher state outside Nano's `/exa` tree.
   The launcher may replace incomplete staging on retry. The VM copies the
   entire legacy tree there while preserving modes, links, timestamps, and
   sparse allocation. It flushes the copy, writes and flushes one versioned
   completion marker, and then renames staging into place atomically. The marker
   travels with the data, so marked staging can be published on retry and a
   marked host destination is authoritative.

   The VM image provides GNU `cp`, which the launcher invokes through the
   runner's existing arbitrary guest-command interface with archive and sparse
   preservation enabled. This avoids adding a migration-specific runner API.

   `podman cp` was considered because the existing overlay migration uses it.
   The guest filesystem copy is preferred because the source and destination
   are both visible in the VM and the copy tool can explicitly preserve sparse
   files. The runner image will provide the required copy behavior as part of
   its contract.

5. Refuse ambiguous data rather than selecting a winner.

   If both the legacy source and an unmarked host destination are populated,
   startup fails with both locations and manual recovery guidance. A marked
   host destination is authoritative and makes later starts idempotent. Marked
   staging is ready to publish, while incomplete staging is replaced from the
   unchanged source.

6. Retain a recoverable guest backup until host startup succeeds.

   The legacy tree remains untouched while copying and while Nano first starts
   from the host. After the database becomes ready, the legacy tree is renamed
   to a migration backup outside `/var/lib/exa`, then host data and the backup
   rename are flushed before startup succeeds. This prevents an older launcher
   from silently starting a stale database at the old path while leaving a
   manual rollback source on the VM disk. Destroy removes the backup with the
   VM runtime.

7. Keep `data_size_gb` for the macOS VM runtime disk.

   The setting continues to size the disk mounted at `/var`, which Podman still
   needs for container images. Its user-facing description becomes "VM runtime
   disk size in GB for Podman images and runtime state," because the host
   filesystem now determines the space available to `/exa`.

8. Treat Windows passthrough as a supported storage contract.

   Windows keeps the existing host directory and Podman-machine passthrough.
   Tests must prove that host-created and Nano-created files are mutually
   visible, database state survives normal and forced lifecycle recovery, and
   destroy removes deployment-owned data without removing the shared Podman
   machine. Performance is observed for diagnostics, not used as a release
   threshold.

## Risks / Trade-offs

- **[Risk] VirtioFS and Windows passthrough have slower or different filesystem
  semantics than native ext4.** → Keep durability flushes, exercise restart and
  forced recovery on both platforms, and retain platform-specific diagnostics.
- **[Risk] Copying sparse database files expands them and exhausts host
  storage.** → Use a copy operation with explicit sparse-file preservation and
  verify allocated size in migration tests.
- **[Risk] The host runs out of space during migration.** → Copy into staging,
  leave the source untouched, and report both paths so the user can free space
  and retry.
- **[Risk] Migration is interrupted.** → Keep the source unchanged until host
  startup succeeds. Replace incomplete staging, publish marked staging, and
  reuse a marked host destination on retry.
- **[Risk] A user has independently populated both locations.** → Refuse the
  migration without changing either tree.
- **[Risk] Downgrading starts a stale guest copy.** → Move the legacy tree away
  from `/var/lib/exa` only after successful host startup and document manual
  rollback from the retained backup.
- **[Trade-off] The VM data disk remains even though Nano data moves out.** → It
  continues to provide writable `/var` and persistent Podman runtime state.

## Migration Plan

1. Start the existing VM with its current data disk and managed host share.
2. Refuse conflicting legacy and unmarked host data.
3. Copy legacy data through reserved staging and publish the marked tree.
4. Start Nano from host storage and wait for database readiness.
5. Retain the legacy tree as a backup and flush the completed state.
6. On retry, replace incomplete staging or resume from marked staging or host
   data.

Before the first successful host-backed start, retry replaces incomplete staging
from the retained guest tree. Afterwards, manual rollback requires stopping Nano
and restoring the retained guest backup to `/var/lib/exa`. A normal deployment
destroy removes host data, the VM disk, and any migration backup.

Fresh macOS deployments create the managed host alias and never put Nano data
at `/var/lib/exa`. Linux and Windows deployments retain their current host-side
data in place without copying.
