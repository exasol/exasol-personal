# Directory mutex

## Overview

This package locks a directory across processes with shared and exclusive advisory file locks of the operating system: `flock` on Linux and macOS and `LockFileEx` on Windows. A lock belongs to an open lock file, so the operating system releases it when the holder releases it, closes the file, or exits, including after a crash or kill. Interrupt handlers do not release locks; a lock stays held until the operation returns or the process exits.

## Lock file location

On Linux and macOS, the lock file is `.exasol-launcher.lock` inside the directory. It moves with the directory when the directory is renamed, so a lock held across a rename stays held at the new path.

On Windows, a directory cannot be renamed while a file inside it is open, and startup migration renames locked deployment directories. Lock files therefore live in the launcher's `locks` directory under its durable root, named after the volume serial number and file index of the locked directory. Both survive a rename within the volume, so the lock also follows the directory. These lock files are empty and are not removed.

Lock files stay in place after release. Directory cleanup must keep them (see `IsLockFileName`), and removing a locked directory goes through `RemoveDirectory` while holding the exclusive lock.

## Acquire and release

Acquisition opens the lock file and attempts a non-blocking lock every 200 ms until it succeeds, the context is canceled or reaches its deadline, or the default acquire timeout of one second expires when the context has no deadline. A timeout returns `ErrAcquireTimeout`. `AcquireExclusiveIfUnlocked` makes a single attempt and reports whether it acquired the lock.

After locking, acquisition checks that the lock file still belongs to the directory at the path. If the directory was renamed or removed between opening and locking, the lock is dropped: acquisition returns an error matching `os.ErrNotExist` when the path is gone, and retries when another directory took its place.

Release unlocks before closing the lock file, because a child process forked meanwhile shares the open file until it executes. A handle holds at most one lock at a time.

On Windows, acquisition also retries when a file operation fails because another process briefly holds a conflicting handle, such as a concurrent rename or a virus scanner. If that error persists until an acquire deadline, acquire returns it instead of a timeout, because the lock may never have been held.

## Status

`Status` probes the lock with non-blocking attempts. An operating system lock does not reveal its holders, so the status reports only whether the directory is locked and in which mode, not how many shared holders exist. The probe briefly takes the lock itself when the directory is unlocked, so a concurrent non-blocking attempt can fail spuriously.

## Compatibility with older launchers

Older launchers locked a directory with one marker file in it: `.dirmutex.exclusive` or `.dirmutex.shared.<n>`. Such a marker can stay behind when an older launcher is terminated. This package never writes markers, but it treats an existing marker as a lock held by an older launcher: acquisition waits for it to disappear, and `Status` reports it with its path. `ClearLock`, used by `exasol diag unlock` and `exasol cache unlock`, removes markers and returns `ErrHeldByProcess` while a running launcher holds the operating system lock, which cannot be cleared from outside.

Older launchers do not see the operating system lock, so an older and a newer launcher running at the same time on the same directory do not exclude each other.

## Limits

Locks are advisory and only exclude processes that use this package. On Windows, they are per user, because the lock files live in the user's profile. On NFS, `flock` is emulated with byte-range locks that depend on the server's lock manager; with the `nolock` mount option, locks only exclude processes on the same client. Those byte-range locks belong to the process rather than the open file, so on NFS two handles in one process do not exclude each other, and closing any handle on the lock file drops the process's lock. Deployment directories on NFS are therefore not supported for concurrent use.
