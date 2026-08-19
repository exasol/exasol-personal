## Context

See `proposal.md` for motivation. Startup currently treats any adopted legacy container name as proof that `/exa` must be migrated, even though v2.2.0 already mounted `/var/lib/exa` persistently. Podman inspect exposes both the source and destination of each mount, while the existing check only reads the destination.

Standalone macOS deployment tests also provision directly rather than using the shared deployment fixture, so they own cleanup on every exit path.

Before the shared Podman refactor, `init-db.sh` ran `sync` after loading and tagging the Nano image. The shared installer initially had no equivalent boundary, so killing the macOS VM daemon after a successful install could expose dirty Podman layers. Separately, Nano can report database readiness before all startup files are durably committed, which the improper-shutdown test exposes when it immediately kills the VM. Automatic Podman repair or reset is not safe because Linux uses the user's host store and a future macOS VM may host more than one deployment.

## Goals / Non-Goals

**Goals:**

- Base migration on the existing `/exa` storage mapping rather than the container name.
- Preserve the staged, non-overwriting migration path for overlay and differently mounted data.
- Make standalone macOS tests validate the current deployment contract and always clean up.
- Restore non-destructive storage durability boundaries on Linux and macOS.

**Non-Goals:**

- Change public CLI behavior or deployment file formats.
- Change destroy target resolution or the VM runner implementation.
- Recover automatically from interruption before a synchronization boundary completes.

## Decisions

### Compare both mount destination and source

Parse `Source` and `Destination` from Podman inspect. A `/exa` mount whose cleaned source equals the configured persistent data directory is already migrated and startup can adopt it in place. A missing `/exa` mount or a different source continues through staged migration.

Container naming is not storage evidence, so the forced migration mode is removed. Keeping a name-based override was considered but would retain the v2.2.0 regression.

### Keep migration refusal and staging semantics unchanged

For data that still requires migration, retain the populated-destination refusal, stop/copy/stage/rename sequence, and recoverable failure behavior. This confines the fix to migration selection.

### Use unconditional test teardown

Wrap each standalone macOS VM lifecycle in `try/finally` and invoke explicit deployment-directory destruction from the cleanup path. Assertions cover loopback connection metadata, shell support, absence of nodes, and absence of SSH transport metadata rather than copied runner or key files.

### Synchronize through the execution environment

Make synchronization an execution-environment operation. The direct environment runs host `sync`, while the macOS runner environment uses the existing `run -- sync` command inside the VM. The shared installer invokes it after Nano and SLC image preparation and before creating the container.

### Isolate the Nano startup durability workaround

Both local runtimes expose `WorkaroundNanoStartupDurability` to deployment orchestration. The deliberately specific name records that this is not a general runtime capability: it compensates for Nano not durably committing all startup files. After the database accepts connections, the workaround synchronizes the execution environment before reporting success.

Keeping this operation distinct from the general execution-environment synchronization contract makes its ownership and removal condition explicit. It can be deleted once Nano commits its startup files durably without changing the runtime's core lifecycle abstraction.

### Do not repair or reset Podman automatically

Startup does not run Podman-wide repair/reset commands and does not persist a Podman durability marker. A failure before synchronization remains a startup failure for diagnosis rather than authorizing deletion from a potentially shared store. The existing Nano initial-create and runner-version markers remain unchanged because they serve separate, scoped purposes.

## Risks / Trade-offs

- [Podman reports a syntactically different source path for the same directory] → Compare cleaned paths and cover the expected runtime path in unit and historical update tests.
- [Cleanup failure hides the original test failure] → Preserve the original failure while still attempting cleanup, following the established historical update test pattern.
- [Host synchronization adds startup latency on Linux] → Keep the calls at the image-preparation boundary and the temporary Nano-workaround boundary, and fail rather than report success when synchronization fails.
- [A process is killed before synchronization completes] → Provide no automatic recovery guarantee and preserve diagnostic evidence instead of mutating the whole Podman store.

## Migration Plan

Ship the mount-aware startup logic with no state conversion. Existing v2.1.0 deployments migrate as before, while v2.2.0 deployments are adopted in place. Rollback requires no data-format reversal.
