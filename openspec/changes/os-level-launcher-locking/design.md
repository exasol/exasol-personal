## Context

Today `internal/directorymutex` implements shared and exclusive locks with marker files inside the locked directory, and `internal/deploy` wraps about 30 operations in `withDeploymentSharedLock` or `withDeploymentExclusiveLock`. The runtime artifact cache and the startup migration use the same mutex directly. The closed draft PR #533 replaced the markers with operating system locks but kept the call sites; this design replaces that approach and reuses its lock primitive. PR #532 (Windows retries, migration under the deployment lock) is already merged.

Constraints: Linux, macOS, and Windows; unit tests run only on Linux in CI; launchers of older versions may still run during the 2.5.0 upgrade; startup migration runs before Cobra parses flags.

## Goals / Non-Goals

**Goals:**
- Locking is an add-on: one package owns all lock mechanics and messages; commands opt in declaratively; deployment operations contain no locking.
- No lock outlives its holder; taking a lock is atomic; a lock follows its directory through a rename.
- In-use errors name the holders; unsupported filesystems and older-version locks produce actionable errors.
- Locking can be disabled per invocation.
- The add-on is testable without commands or deployments.

**Non-Goals:**
- Making locks work across machines on NFS without a lock service.
- Excluding older launcher versions, which only know marker files.
- Changing which operations count as changing or reading a deployment.

## Decisions

### D1: Operating system file locks on a lock file
`internal/filelock` wraps non-blocking shared and exclusive locks: `flock` on Linux and macOS and `LockFileEx` on Windows. Callers poll with the existing retry interval, so timeouts and context cancellation keep their current behavior. Release unlocks before closing, so a child process forked at that moment cannot keep the lock.
*Alternatives:* `fcntl` byte-range locks are per process, so two handles in one process would not exclude each other. Locking the directory handle itself works with `flock` on Linux and macOS but not on Windows, where `LockFileEx` takes only files.

### D2: Lock files outside the locked directory, keyed by directory identity
A lock file lives at `<launcher locks directory>/<key>.lock`. The key is the directory's identity: device and inode on Unix, volume serial number and file index on Windows. A rename keeps the identity, so a deployment's lock follows it through migration. After locking, the holder checks that the path still resolves to a directory with that identity. If the directory is gone, acquisition reports it as missing; if it was replaced, acquisition retries. When a holder releases the exclusive lock of a directory that no longer exists, it removes the lock file.
*Alternatives:* A lock file inside the directory blocks renames on Windows, pollutes directories selected by mistake, and needs special cases in `init`, `remove`, and the cache. A key derived from the path breaks on rename and on path aliases such as symbolic links or case-insensitive names.

### D3: Holder records for in-use errors
After acquiring, a holder writes a record next to the lock file with its command path (for example `exasol shell`), process ID, and start time, and removes it on release. Only the command path is recorded, not the arguments, so secrets on the command line never reach disk. In-use errors list the records whose processes still run; records of ended processes are ignored and pruned. Records never take part in the locking decision, so a reused process ID can at worst produce a misleading name, never a wrong lock state.
*Alternative:* writing the holder into the lock file itself does not work for several shared holders.

### D4: Commands declare their access; the command layer locks
`requireDeploymentLock(cmd, locking.Shared)` or `locking.Exclusive` records the access as a command annotation, following `requireInitializedDeploymentDir`. The root `PersistentPreRunE` takes the lock once, after deployment resolution and compatibility checks and before the deployment log session starts. `Execute` releases it right after the command returns, before the post-command version check. `init` and `install` declare that the directory may be created first. Operations in `internal/deploy` assume the caller holds the declared access and contain no locking.

Places that are not "one command, one deployment" call `locking.With(ctx, dir, mode, fn)` from the command layer or their own package: the per-entry status in `deployments list`, `exasol status` (both report in-use as `operation_in_progress`), the post-command version check, the startup migration, and the runtime artifact cache. A test walks the command tree and requires every runnable command to declare its access explicitly, either as a lock mode or as "no deployment lock". A new command therefore cannot forget it, and turning locking on or off for a command is a one-line change.
*Alternative:* keeping locks inside operations, as today, scatters the policy, nests locks in one process, and makes `--no-lock` a parameter of every operation.

### D5: One locker, chosen once per process
The locking package exposes a `Locker` with two implementations: the operating system locker and a disabled locker that never waits and never locks. The command layer selects one at the start of `Execute` and attaches it to the context, as it already does for the runtime artifact resource manager. Everything that locks takes the locker from the context. Startup migration runs before Cobra parses flags, so `Execute` reads `--no-lock` from the raw arguments, together with `EXASOL_LAUNCHER_NO_LOCK`, the way it already detects help and completion. Cobra registers the same flag so that help and validation show it. The disabled locker prints its warning once per process, as an operational notice.

### D6: Error classification in the add-on
The add-on maps acquisition outcomes to typed errors with complete user messages: in use, with holders, or held by an older version, with the marker path and unlock command; filesystem without lock support (`ENOLCK`, `EOPNOTSUPP`, `ENOTSUP`, Windows `ERROR_NOT_SUPPORTED`), with the `--no-lock` workaround; and directory missing. Transient Windows errors keep the retry from #532. Command code only renders these errors and never builds lock messages itself.

### D7: Older-version markers are read, never written
Existing `.dirmutex.*` markers count as held by an older launcher version. `exasol diag unlock` and `exasol cache unlock` take the exclusive lock without waiting: if a running launcher of this version holds it, they fail with the in-use error; otherwise they remove markers and report whether they removed one. Writing markers as well would bring back stale locks and Windows rename failures, so mixed-version use is documented as unsupported (**BREAKING**).

### D8: Interrupts rely on process exit
No signal handler releases locks. On Ctrl-C the main handler interrupts child processes, waits for them, and exits; the operating system then releases all locks. This keeps the deployment locked while OpenTofu still writes its state.

## Risks / Trade-offs

- [NFS without a working lock service, or mounted with `nolock`] → errors explain `--no-lock`; documentation states that deployments on network filesystems support one launcher at a time.
- [Lock files accumulate for directories that disappear without a release, such as a deployment deleted by hand] → each is one empty file and holder records are pruned; a release that finds its directory gone removes the lock file.
- [Directory identity reuse after deletion] → a new directory can share the lock file of a deleted one, which only means they share a lock file, never a wrong lock state.
- [Windows code paths have no Windows unit tests in CI] → only reading the identity and calling `LockFileEx` are Windows-specific; everything else runs on Linux through an injectable identity function; the Windows launcher tests in CI cover the integration.
- [`install` holds the exclusive lock for its whole run] → intended: no other command can interleave between initialization and deployment.
- [Older and newer launchers do not exclude each other] → breaking-change note, migration guidance, and help text say not to run them at the same time.

## Migration Plan

No data migration. Existing markers keep blocking until cleared with the unlock commands. Rolling back to an older launcher is safe: it ignores the new lock files, which hold no state.

## Open Questions

- Acquisition waiting times stay as they are today, short for deployment commands; this change does not introduce waiting with a progress notice for deployment commands.
