## Why

The launcher locks a deployment, the runtime artifact cache, and the startup migration so that concurrent launcher commands do not corrupt state. The current design does that with marker files (`.dirmutex.exclusive`, `.dirmutex.shared.<n>`) inside the locked directory, taken and released by about 30 call sites inside deployment operations. That design fails users in ways that a different design avoids:

| Weakness of marker-file locking | Consequence for users and maintainers | This design |
|---|---|---|
| A marker outlives a launcher that crashes, is killed, or loses its terminal. | Every later command waits and fails with "locked by another operation. Do not use `unlock` unless…". Users must guess whether an unlock is safe. | Operating system file locks end with the process, so no lock outlives its holder and no unlock is needed. |
| Taking a lock is "list the directory, then create or rename a marker", which is not atomic. Exclusive and shared markers have different names. | Two processes can both believe they hold the lock. A marker created while another launcher renames the directory lands in the moved directory and stays there; CI hit this during concurrent migration (PR #532). | One atomic lock call, plus a check after locking that the lock still belongs to the directory at the path. |
| Markers live in the locked directory and are renamed or deleted on every lock change. | Windows refuses these operations while another process briefly holds the file or directory, so commands fail or leave a lock behind. Directories that are not deployments, such as one passed to `--deployment-dir` by mistake, get marker files. | Lock files live in a launcher-owned directory, keyed by the directory's identity, which a rename keeps. Selected directories contain only their own content, and the same code runs on every platform. |
| Locking is woven into about 30 deployment operations as nested callbacks. | Which command holds which lock, and for how long, is only visible by reading every operation. A combined command such as `install` takes and drops locks between its steps. Locks cannot be tested or disabled without the operations, and nested locks in one process can wait on each other. | Commands declare shared or exclusive access in one line. The command layer takes the lock once for the whole command, and deployment operations contain no locking. The locking package is tested on its own. |
| The in-use error cannot say who holds the lock. | Users with several terminals, IDE integrations, or agents cannot find the command to wait for or stop. | Each holder records its command, process ID, and start time, and in-use errors list the running holders. |
| Locking cannot be turned off, and failures of the filesystem's lock support surface as unexplained errors. | Users with homes on NFS or similar filesystems have no workaround. | `--no-lock` and `EXASOL_LAUNCHER_NO_LOCK` run a command without locks, with a warning. Unsupported filesystems produce an error that explains the workaround and its risk. |

Doing this now matters because the launcher path migration in the unreleased 2.5.0 moves deployment directories on the first start after an upgrade, often from several commands at once. That is the situation where marker-file locking fails most visibly.

## What Changes

- Deployment, runtime artifact cache, and startup migration locks are operating system file locks that end with the launcher process, also after a crash or kill.
- Locking becomes a self-contained add-on: commands declare whether they need shared or exclusive access to the selected deployment, and the command layer takes and releases that lock. Deployment operations no longer lock internally.
- An "in use" error names each running launcher command that holds the deployment or cache, with its process ID and start time, and says to wait for it or stop it.
- New global option `--no-lock` and environment variable `EXASOL_LAUNCHER_NO_LOCK` run a command without taking or honoring locks, with a warning on standard error.
- When the filesystem refuses file locks, the error names the path and explains `--no-lock` as an at-your-own-risk workaround.
- Lock files live in a launcher-owned directory instead of deployment directories, so deployment and other selected directories contain only their own content.
- `exasol diag unlock` and `exasol cache unlock` clear only locks left by older launcher versions, report whether they removed one, and refuse while a running launcher of this version holds the lock.
- A launcher waiting for another launcher's startup migration says so.
- **BREAKING**: launchers of this version and older versions do not lock each other out of a deployment or the cache.

## Capabilities

### New Capabilities

- `launcher-locking`: coordination of concurrent launcher processes on a deployment, its user-facing errors, recovery from older-version locks, unsupported filesystems, and disabling locks.

### Modified Capabilities

- `runtime-artifact-cache`: cache unlocking clears only older-version locks and reports its result; lock contention names the holders.
- `launcher-path-migration`: concurrent startup migrations wait for each other with a notice and leave no locks behind.

## Impact

Builds on the merged PR #532 and supersedes the closed draft PR #533; tracked by SPOT-34137 under SPOT-34136. Affects the command layer (root hooks, command annotations, global flags), `internal/deploy` (locking removed from operations), the runtime artifact cache, startup migration, and the diagnostics and cache unlock commands. Replaces `internal/directorymutex` with a new locking package built on a small file lock primitive using `golang.org/x/sys`. Updates user documentation (troubleshooting, glossary, scripting, cache, NFS limits), the CLI reference, and the changelog. No new dependencies.
