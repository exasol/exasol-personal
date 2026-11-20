## 1. Lock primitive

- [ ] 1.1 Keep `internal/filelock`: non-blocking shared and exclusive operating system file locks (`flock`, `LockFileEx`), unlock before close, with conflict and release tests. Commit: `refactor: Add an operating system file lock primitive`.

## 2. Locking add-on

- [ ] 2.1 Add `internal/locking` without callers: directory identity (injectable, Unix and Windows), lock files keyed by identity in the launcher locks directory, the post-lock identity check, removal of lock files of vanished directories, holder records, older-version marker detection and clearing, typed errors with complete messages (in use with holders, older-version marker, unsupported filesystem, missing directory), the transient Windows retry, the `Locker` interface with operating system and disabled implementations, context attachment, and `With`. Unit tests cover each part on Linux, including the Windows layout through the injected identity, rename and replace races, a killed holder subprocess, and holder pruning. Document the package in its own README. Commit: `refactor: Add a locking add-on for launcher directories`.

## 3. Command-level deployment locking

- [ ] 3.1 Add the `requireDeploymentLock` annotation, the root hook that takes the declared lock after resolution and compatibility checks, and the release in `Execute` before the version check. Declare the access of every runnable command, with "no deployment lock" where none applies, and add a test that walks the command tree and requires a declaration. Remove all locking from `internal/deploy`; move `exasol status`, `deployments list`, and the version check onto `locking.With`; render in-use and unsupported-filesystem errors with holders. Update `launcher-locking` launcher tests, troubleshooting, glossary, scripting docs, the CLI reference, and the changelog for the deployment-lock behavior. Commit: `fix: Release deployment locks when the launcher exits`.

## 4. Cache and migration

- [ ] 4.1 Move the runtime artifact cache, `exasol cache unlock`, `exasol diag unlock`, and the startup migration (launcher-wide migration lock with a waiting notice, per-deployment migration lock) onto `internal/locking`. Delete `internal/directorymutex` and the lock-file special cases in `init`, `remove`, and the cache. Unlock commands report whether they removed an older-version lock and fail with the in-use error while a launcher of this version holds the lock. Update cache documentation, help texts, the CLI reference, and the changelog, including the breaking note on mixed launcher versions. Commit: `fix: Coordinate the cache and startup migration with process-bound locks`.

## 5. Disabling locks

- [ ] 5.1 Add the global `--no-lock` option and `EXASOL_LAUNCHER_NO_LOCK`, read before startup migration from the raw arguments and the environment, selecting the disabled locker with a one-time warning naming the source. Add launcher tests, an NFS and network filesystem section in the troubleshooting documentation, the CLI reference, and the changelog. Commit: `feat: Add an option to run without launcher locks`.

## 6. Validation and review

- [ ] 6.1 Verify each implementation commit independently with `task fmt`, `task lint`, and the Go and launcher tests of the packages it touches; run `task all` on the final state. Validation only; no commit.
- [ ] 6.2 Run the locking and concurrent migration tests repeatedly with `-race`; exercise concurrent launcher processes, `SIGKILL` and Ctrl-C during a deploy with a slow engine, an older-version marker, a directory selected by mistake, and `--no-lock`. Validation only; no commit.
- [ ] 6.3 Cross-compile and lint the Windows and macOS code; confirm the Windows launcher tests in CI. Validation only; no commit.
- [ ] 6.4 Obtain an independent code review and a user evaluation of the changed commands and documentation; resolve or disposition findings. Validation only; no commit.

## Delivery sequence

After implementation and verification, sync and archive this change and commit the main specifications and archived artifacts separately as `docs(openspec): Record process-bound launcher locking`. Deliver the series in the SPOT-34137 pull request, reusing the prototype from the closed PR #533 where it fits.
