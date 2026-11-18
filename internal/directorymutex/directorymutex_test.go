// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package directorymutex

import (
	"bufio"
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type mutexFactory func(t *testing.T, path string) *DirectoryMutex

// lockStrategies returns mutex factories for lock files inside the directory
// and for lock files keyed by directory identity, as used on Windows.
func lockStrategies(t *testing.T) map[string]mutexFactory {
	t.Helper()

	factory := func(keyedLockDir string) mutexFactory {
		return func(t *testing.T, path string) *DirectoryMutex {
			t.Helper()

			mutex, err := New(path)
			require.NoError(t, err)
			mutex.keyedLockDir = keyedLockDir
			t.Cleanup(func() { _ = mutex.Release() })

			return mutex
		}
	}
	strategies := map[string]mutexFactory{"keyed lock files": factory(t.TempDir())}
	// Windows cannot rename a directory holding an open lock file.
	if runtime.GOOS != "windows" {
		strategies["lock file inside"] = factory("")
	}

	return strategies
}

func requireStatus(t *testing.T, mutex *DirectoryMutex, mode string) {
	t.Helper()

	status, err := mutex.Status()
	require.NoError(t, err)
	require.Equal(t, mode != "", status.Locked, status)
	require.Equal(t, mode, status.Mode)
}

func TestNewRejectsNonDirectory(t *testing.T) {
	t.Parallel()

	// Given a file path (not a directory)
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))

	// When creating a DirectoryMutex with that path
	_, err := New(file)

	// Then ErrPathNotDirectory is returned
	require.ErrorIs(t, err, ErrPathNotDirectory)
}

func TestIsLockFileNameRecognizesLockFilesAndOlderMarkers(t *testing.T) {
	t.Parallel()

	for input, match := range map[string]bool{
		lockFileName:         true,
		exclusiveMarkerName:  true,
		".dirmutex.shared.3": true,
		"notes.txt":          false,
	} {
		// When
		got := IsLockFileName(input)

		// Then
		require.Equal(t, match, got, input)
	}
}

func TestExclusiveLockBlocksOtherHandlesUntilTimeout(t *testing.T) {
	t.Parallel()

	for name, newMutex := range lockStrategies(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// Given an exclusive lock is held
			dir := t.TempDir()
			holder := newMutex(t, dir)
			require.NoError(t, holder.AcquireExclusive(context.Background()))
			other := newMutex(t, dir)

			// When another handle tries to acquire a shared lock with a short deadline
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			err := other.AcquireShared(ctx)

			// Then the acquisition times out and the exclusive lock stays
			require.ErrorIs(t, err, context.DeadlineExceeded)
			require.ErrorIs(t, err, ErrAcquireTimeout)
			requireStatus(t, other, modeExclusiveString)

			// When the holder releases
			require.NoError(t, holder.Release())

			// Then the other handle acquires the lock
			require.NoError(t, other.AcquireShared(context.Background()))
		})
	}
}

func TestSharedLocksCoexistUntilTheLastRelease(t *testing.T) {
	t.Parallel()

	for name, newMutex := range lockStrategies(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// Given two shared lock holders on the same directory
			dir := t.TempDir()
			first := newMutex(t, dir)
			second := newMutex(t, dir)
			require.NoError(t, first.AcquireShared(context.Background()))
			require.NoError(t, second.AcquireShared(context.Background()))
			observer := newMutex(t, dir)

			// Then the directory is locked in shared mode
			requireStatus(t, observer, modeSharedString)

			// When one holder releases
			require.NoError(t, second.Release())

			// Then it stays locked
			requireStatus(t, observer, modeSharedString)

			// When the last holder releases
			require.NoError(t, first.Release())

			// Then it is unlocked
			requireStatus(t, observer, "")
		})
	}
}

func TestAcquireTwiceOnOneHandleFails(t *testing.T) {
	t.Parallel()

	// Given a handle that holds a lock
	mutex, err := New(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, mutex.AcquireShared(context.Background()))
	t.Cleanup(func() { _ = mutex.Release() })

	// When it acquires again
	err = mutex.AcquireShared(context.Background())

	// Then it refuses
	require.ErrorIs(t, err, ErrAlreadyHeld)
}

func TestAcquireExclusiveIfUnlockedAcquiresOnlyAnUnlockedDirectory(t *testing.T) {
	t.Parallel()

	for name, newMutex := range lockStrategies(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// Given one directory with a shared lock and another unlocked one
			lockedDir := t.TempDir()
			require.NoError(t, newMutex(t, lockedDir).AcquireShared(context.Background()))
			locked := newMutex(t, lockedDir)
			unlocked := newMutex(t, t.TempDir())

			// When each tries to acquire an exclusive lock without waiting
			lockedAcquired, lockedErr := locked.AcquireExclusiveIfUnlocked()
			unlockedAcquired, unlockedErr := unlocked.AcquireExclusiveIfUnlocked()

			// Then only the unlocked directory is acquired and the shared lock is untouched
			require.NoError(t, lockedErr)
			require.False(t, lockedAcquired)
			require.NoError(t, unlockedErr)
			require.True(t, unlockedAcquired)
			requireStatus(t, locked, modeSharedString)
			requireStatus(t, unlocked, modeExclusiveString)
		})
	}
}

func TestLockMovesWithARenamedDirectory(t *testing.T) {
	t.Parallel()

	for name, newMutex := range lockStrategies(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// Given an exclusively locked directory
			root := t.TempDir()
			oldPath := filepath.Join(root, "old")
			newPath := filepath.Join(root, "new")
			require.NoError(t, os.Mkdir(oldPath, 0o700))
			holder := newMutex(t, oldPath)
			require.NoError(t, holder.AcquireExclusive(context.Background()))

			// When the holder renames it
			require.NoError(t, os.Rename(oldPath, newPath))

			// Then it stays locked at the new path until the holder releases
			moved := newMutex(t, newPath)
			requireStatus(t, moved, modeExclusiveString)
			require.NoError(t, holder.Release())
			requireStatus(t, moved, "")
		})
	}
}

func TestAcquireOnADirectoryRenamedWhileLockingLeavesItUnlocked(t *testing.T) {
	t.Parallel()

	for name, newMutex := range lockStrategies(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// Given a directory that another launcher renames after this one opened its lock file
			root := t.TempDir()
			oldPath := filepath.Join(root, "old")
			newPath := filepath.Join(root, "new")
			require.NoError(t, os.Mkdir(oldPath, 0o700))
			mutex := newMutex(t, oldPath)
			mutex.beforeLock = func() {
				require.NoError(t, os.Rename(oldPath, newPath))
			}

			// When it acquires the lock without waiting
			acquired, err := mutex.AcquireExclusiveIfUnlocked()

			// Then it reports the directory as gone and the moved directory is unlocked
			require.ErrorIs(t, err, os.ErrNotExist)
			require.False(t, acquired)
			requireStatus(t, newMutex(t, newPath), "")
		})
	}
}

func TestAcquireOnADirectoryReplacedWhileLockingLocksTheReplacement(t *testing.T) {
	t.Parallel()

	for name, newMutex := range lockStrategies(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// Given a directory replaced by a new one after its lock file was opened
			root := t.TempDir()
			path := filepath.Join(root, "dir")
			movedPath := filepath.Join(root, "moved")
			require.NoError(t, os.Mkdir(path, 0o700))
			mutex := newMutex(t, path)
			mutex.beforeLock = func() {
				mutex.beforeLock = nil
				require.NoError(t, os.Rename(path, movedPath))
				require.NoError(t, os.Mkdir(path, 0o700))
			}

			// When it acquires the lock
			err := mutex.AcquireExclusive(context.Background())

			// Then it holds the lock of the directory now at the path
			require.NoError(t, err)
			requireStatus(t, newMutex(t, path), modeExclusiveString)
			requireStatus(t, newMutex(t, movedPath), "")
		})
	}
}

func TestRemoveDirectoryUnderTheLockFailsWaitingAcquires(t *testing.T) {
	t.Parallel()

	for name, newMutex := range lockStrategies(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// Given an exclusively locked directory and another handle on it
			dir := filepath.Join(t.TempDir(), "dir")
			require.NoError(t, os.Mkdir(dir, 0o700))
			holder := newMutex(t, dir)
			require.NoError(t, holder.AcquireExclusive(context.Background()))
			waiter := newMutex(t, dir)

			// When the holder removes the directory and releases
			require.NoError(t, RemoveDirectory(dir))
			require.NoError(t, holder.Release())

			// Then the other handle reports the directory as gone
			require.ErrorIs(t, waiter.AcquireExclusive(context.Background()), os.ErrNotExist)
			require.NoDirExists(t, dir)
		})
	}
}

func TestRemoveDirectoryWhileAnotherLauncherWaitsLeavesNothingBehind(t *testing.T) {
	t.Parallel()

	for name, newMutex := range lockStrategies(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			for range 50 {
				// Given an exclusively locked directory and a launcher waiting for it
				parent := t.TempDir()
				dir := filepath.Join(parent, "dir")
				require.NoError(t, os.Mkdir(dir, 0o700))
				holder := newMutex(t, dir)
				require.NoError(t, holder.AcquireExclusive(context.Background()))
				waiter := newMutex(t, dir)
				waited := make(chan error, 1)
				go func() { waited <- waiter.AcquireShared(context.Background()) }()

				// When the holder removes the directory and releases
				require.NoError(t, RemoveDirectory(dir))
				require.NoError(t, holder.Release())

				// Then the waiter finds it gone and nothing is left in the parent
				require.ErrorIs(t, <-waited, os.ErrNotExist)
				entries, err := os.ReadDir(parent)
				require.NoError(t, err)
				require.Empty(t, entries)
			}
		})
	}
}

func TestOlderLauncherMarkerHoldsTheLockUntilCleared(t *testing.T) {
	t.Parallel()

	for marker, mode := range map[string]string{
		exclusiveMarkerName:  modeExclusiveString,
		".dirmutex.shared.2": modeSharedString,
	} {
		t.Run(marker, func(t *testing.T) {
			t.Parallel()

			// Given a marker file left by an older launcher
			dir := t.TempDir()
			markerPath := filepath.Join(dir, marker)
			require.NoError(t, os.WriteFile(markerPath, nil, 0o600))
			mutex, err := New(dir)
			require.NoError(t, err)
			t.Cleanup(func() { _ = mutex.Release() })

			// When acquiring the lock
			acquired, ifUnlockedErr := mutex.AcquireExclusiveIfUnlocked()
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			sharedErr := mutex.AcquireShared(ctx)
			status, statusErr := mutex.Status()

			// Then the marker holds it
			require.NoError(t, ifUnlockedErr)
			require.False(t, acquired)
			require.ErrorIs(t, sharedErr, ErrAcquireTimeout)
			require.NoError(t, statusErr)
			require.Equal(t, Status{Locked: true, Mode: mode, MarkerPath: markerPath}, status)

			// When the lock is cleared
			require.NoError(t, mutex.ClearLock())

			// Then the marker is gone and the lock can be acquired
			require.NoFileExists(t, markerPath)
			require.NoError(t, mutex.AcquireExclusive(context.Background()))
		})
	}
}

func TestClearLockReportsALockHeldByARunningProcess(t *testing.T) {
	t.Parallel()

	for name, newMutex := range lockStrategies(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// Given an exclusive lock is held
			dir := t.TempDir()
			require.NoError(t, newMutex(t, dir).AcquireExclusive(context.Background()))
			other := newMutex(t, dir)

			// When clearing the lock
			err := other.ClearLock()

			// Then it reports the holder and the lock stays
			require.ErrorIs(t, err, ErrHeldByProcess)
			requireStatus(t, other, modeExclusiveString)
		})
	}
}

const helperProcessEnv = "DIRECTORYMUTEX_HELPER_PROCESS"

// TestHelperProcessHoldsExclusiveLock locks its working directory when run as
// a helper process.
func TestHelperProcessHoldsExclusiveLock(t *testing.T) {
	t.Parallel()

	if os.Getenv(helperProcessEnv) == "" {
		t.Skip("runs only as a helper process")
	}
	mutex, err := New(".")
	require.NoError(t, err)
	require.NoError(t, mutex.AcquireExclusive(context.Background()))
	_, _ = os.Stdout.WriteString("locked\n")
	time.Sleep(time.Minute)
}

func TestKilledProcessLeavesTheDirectoryUnlocked(t *testing.T) {
	t.Parallel()

	// Given another process holds the exclusive lock
	dir := t.TempDir()
	//nolint:gosec // Re-runs this test binary as the helper process.
	cmd := exec.CommandContext(
		t.Context(), os.Args[0], "-test.run=^TestHelperProcessHoldsExclusiveLock$",
	)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), helperProcessEnv+"=1")
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	scanner := bufio.NewScanner(stdout)
	require.True(t, scanner.Scan(), "helper process did not report its lock")
	require.Equal(t, "locked", scanner.Text())
	mutex, err := New(dir)
	require.NoError(t, err)
	requireStatus(t, mutex, modeExclusiveString)

	// When it is killed
	require.NoError(t, cmd.Process.Kill())
	_ = cmd.Wait()

	// Then the directory is unlocked
	require.NoError(t, mutex.AcquireExclusive(context.Background()))
	require.NoError(t, mutex.Release())
}

// newPermissionDeniedMutex returns a mutex on a read-only directory that treats
// the resulting permission errors like Windows' transient handle conflicts.
func newPermissionDeniedMutex(t *testing.T) (*DirectoryMutex, func()) {
	t.Helper()
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("relies on POSIX directory permissions enforced for a non-root user")
	}
	dir := t.TempDir()
	//nolint:gosec // A read-only directory makes lock file creation fail.
	require.NoError(t, os.Chmod(dir, 0o500))
	allow := func() {
		_ = os.Chmod(dir, 0o700) //nolint:gosec // Restores the test directory.
	}
	t.Cleanup(allow)
	mutex, err := New(dir)
	require.NoError(t, err)
	mutex.isTransient = func(err error) bool { return errors.Is(err, fs.ErrPermission) }
	t.Cleanup(func() { _ = mutex.Release() })

	return mutex, allow
}

func TestAcquireRetriesTransientFileErrorsUntilTheyClear(t *testing.T) {
	t.Parallel()

	// Given a directory that refuses lock file creation for a moment
	mutex, allow := newPermissionDeniedMutex(t)
	time.AfterFunc(3*defaultRetryInterval, allow)

	// When acquiring with time to spare
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := mutex.AcquireExclusive(ctx)

	// Then the lock is acquired once the error clears
	require.NoError(t, err)
}

func TestAcquireReportsATransientFileErrorThatOutlastsTheTimeout(t *testing.T) {
	t.Parallel()

	// Given a directory that keeps refusing lock file creation
	mutex, _ := newPermissionDeniedMutex(t)

	// When acquiring until the deadline
	ctx, cancel := context.WithTimeout(context.Background(), 3*defaultRetryInterval)
	defer cancel()
	err := mutex.AcquireExclusive(ctx)

	// Then the file error is reported instead of a held lock
	require.ErrorIs(t, err, fs.ErrPermission)
	require.NotErrorIs(t, err, ErrAcquireTimeout)
}
