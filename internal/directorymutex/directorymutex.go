// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package directorymutex

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/exasol/exasol-personal/internal/filelock"
)

var (
	ErrAcquireTimeout   = errors.New("lock acquire timed out")
	ErrPathNotDirectory = errors.New("path is not a directory")
	ErrAlreadyHeld      = errors.New("directory mutex handle already holds a lock")
	ErrHeldByProcess    = errors.New(
		"the directory is locked by a running launcher process and unlocks when it exits",
	)
)

const (
	modeSharedString    = "shared"
	modeExclusiveString = "exclusive"
)

// Older launchers lock with these marker files. Their names must never match
// lockFileName, or those launchers fail on the directory.
const (
	markerStem          = ".dirmutex"
	exclusiveMarkerName = markerStem + ".exclusive"
	sharedMarkerPrefix  = markerStem + ".shared."
)

const lockFileName = ".exasol-launcher.lock"

const (
	defaultAcquireTimeout = 1 * time.Second
	defaultRetryInterval  = 200 * time.Millisecond
	// A lock file that changes while being locked belongs to a directory that
	// was just renamed or removed, so a few attempts settle on its fate.
	maxChangedAttempts = 3
	lockDirPerm        = 0o700
	lockFilePerm       = 0o600
)

var errDirectoryChanged = errors.New("directory changed while locking")

// Status reports the observed lock state in a directory. MarkerPath is set
// when an older launcher's marker file holds the lock.
type Status struct {
	Locked     bool
	Mode       string
	MarkerPath string
}

// DirectoryMutex locks a directory across processes with an operating system
// file lock, which the operating system releases when the holder exits.
// A handle holds at most one lock at a time.
type DirectoryMutex struct {
	path string
	// keyedLockDir holds lock files named by directory identity instead of a
	// lock file inside the directory; empty selects the lock file inside.
	keyedLockDir string
	isTransient  func(error) bool
	beforeLock   func()

	mu   sync.Mutex
	held *os.File
}

// IsLockFileName reports whether name is a lock file of this package or a
// marker of an older launcher, which directory cleanup must leave in place.
func IsLockFileName(name string) bool {
	return name == lockFileName || isMarkerName(name)
}

func isMarkerName(name string) bool {
	return name == exclusiveMarkerName || strings.HasPrefix(name, sharedMarkerPrefix)
}

// New validates path and prepares a directory mutex handle.
func New(path string) (*DirectoryMutex, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, ErrPathNotDirectory
	}
	keyedLockDir, err := defaultKeyedLockDir()
	if err != nil {
		return nil, err
	}

	return &DirectoryMutex{
		path:         path,
		keyedLockDir: keyedLockDir,
		isTransient:  isTransientFileError,
	}, nil
}

// RemoveDirectory removes the empty directory path together with its lock
// file. The caller holds the exclusive lock. The directory first moves to a
// hidden sibling, so launchers waiting for the lock find it gone instead of
// recreating a lock file that keeps it from being removed. A waiter whose
// open raced the move can still recreate the lock file in the moved
// directory, so removal retries while that file is all that is left.
func RemoveDirectory(path string) error {
	removing := filepath.Join(
		filepath.Dir(path),
		fmt.Sprintf(".%s.removing-%d", filepath.Base(path), os.Getpid()),
	)
	if err := os.Rename(path, removing); errors.Is(err, os.ErrNotExist) {
		return err
	} else if err != nil {
		removing = path
	}

	for attempt := 1; ; attempt++ {
		if err := os.Remove(filepath.Join(removing, lockFileName)); err != nil &&
			!errors.Is(err, os.ErrNotExist) {
			return err
		}
		err := os.Remove(removing)
		if err == nil || attempt == removeDirectoryAttempts || !onlyLockFileLeft(removing) {
			return err
		}
	}
}

const removeDirectoryAttempts = 5

func onlyLockFileLeft(path string) bool {
	entries, err := os.ReadDir(path)

	return err == nil && len(entries) == 1 && entries[0].Name() == lockFileName
}

// AcquireShared acquires a shared lock.
func (m *DirectoryMutex) AcquireShared(ctx context.Context) error {
	return m.acquire(ctx, false)
}

// AcquireExclusive acquires an exclusive lock.
func (m *DirectoryMutex) AcquireExclusive(ctx context.Context) error {
	return m.acquire(ctx, true)
}

// AcquireExclusiveIfUnlocked acquires an exclusive lock only if the directory is
// unlocked right now, and reports whether it did.
func (m *DirectoryMutex) AcquireExclusiveIfUnlocked() (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.held != nil {
		return false, ErrAlreadyHeld
	}
	file, err := m.tryLock(true)
	if errors.Is(err, filelock.ErrWouldBlock) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	m.held = file

	return true, nil
}

// Release releases the lock this handle holds, if any.
func (m *DirectoryMutex) Release() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.held == nil {
		return nil
	}
	file := m.held
	m.held = nil

	return releaseLockFile(file)
}

func (m *DirectoryMutex) WithShared(ctx context.Context, arg any, function func(any) error) error {
	return m.withLock(ctx, arg, function, false)
}

func (m *DirectoryMutex) WithExclusive(
	ctx context.Context,
	arg any,
	function func(any) error,
) error {
	return m.withLock(ctx, arg, function, true)
}

// ClearLock removes marker files that older launchers left behind. A lock
// held by a running launcher cannot be cleared, so it reports
// ErrHeldByProcess for one.
func (m *DirectoryMutex) ClearLock() error {
	entries, err := os.ReadDir(m.path)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !isMarkerName(entry.Name()) {
			continue
		}
		err := os.Remove(filepath.Join(m.path, entry.Name()))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}

	mode, err := m.probe()
	if err != nil {
		return err
	}
	if mode != "" {
		return ErrHeldByProcess
	}

	return nil
}

// Status reports the current observed lock state. A probe cannot count shared
// holders, and it briefly takes the lock itself when the directory is
// unlocked.
func (m *DirectoryMutex) Status() (Status, error) {
	marker, err := m.findMarker()
	if err != nil {
		return Status{}, err
	}
	if marker != "" {
		mode := modeSharedString
		if filepath.Base(marker) == exclusiveMarkerName {
			mode = modeExclusiveString
		}

		return Status{Locked: true, Mode: mode, MarkerPath: marker}, nil
	}

	mode, err := m.probe()
	if err != nil {
		return Status{}, err
	}

	return Status{Locked: mode != "", Mode: mode}, nil
}

// probe returns the mode of the lock that another handle holds, or "" if
// there is none.
func (m *DirectoryMutex) probe() (string, error) {
	file, _, err := m.openLockFile(false)
	if errors.Is(err, os.ErrNotExist) {
		if _, statErr := os.Stat(m.path); statErr != nil {
			return "", statErr
		}

		return "", nil
	} else if err != nil {
		return "", err
	}
	defer file.Close()

	for _, attempt := range []struct {
		exclusive bool
		heldMode  string
	}{
		{exclusive: true, heldMode: ""},
		{exclusive: false, heldMode: modeSharedString},
	} {
		err := filelock.TryLock(file, attempt.exclusive)
		if err == nil {
			return attempt.heldMode, filelock.Unlock(file)
		}
		if !errors.Is(err, filelock.ErrWouldBlock) {
			return "", err
		}
	}

	return modeExclusiveString, nil
}

func (m *DirectoryMutex) acquire(ctx context.Context, exclusive bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.held != nil {
		return ErrAlreadyHeld
	}

	lockCtx, cancel := withTimeoutIfMissing(ctx, defaultAcquireTimeout)
	defer cancel()

	var transientErr error
	for {
		if err := lockCtx.Err(); err != nil {
			return wrapAcquireError(err, transientErr)
		}

		file, err := m.tryLock(exclusive)
		if err == nil {
			m.held = file
			return nil
		}
		transientErr = m.transientError(err)
		if transientErr == nil && !errors.Is(err, filelock.ErrWouldBlock) {
			return err
		}

		if err := sleepWithContext(lockCtx, defaultRetryInterval); err != nil {
			return wrapAcquireError(err, transientErr)
		}
	}
}

// tryLock returns the locked lock file, or filelock.ErrWouldBlock while
// another handle or an older launcher's marker holds the lock. It returns
// an error matching os.ErrNotExist once the directory is gone.
func (m *DirectoryMutex) tryLock(exclusive bool) (*os.File, error) {
	for range maxChangedAttempts {
		file, err := m.tryLockOnce(exclusive)
		if !errors.Is(err, errDirectoryChanged) {
			return file, err
		}
		if _, err := os.Stat(m.path); err != nil {
			return nil, err
		}
	}

	return nil, filelock.ErrWouldBlock
}

func (m *DirectoryMutex) tryLockOnce(exclusive bool) (*os.File, error) {
	file, unchanged, err := m.openLockFile(true)
	if err != nil {
		return nil, err
	}
	if m.beforeLock != nil {
		m.beforeLock()
	}

	if err := filelock.TryLock(file, exclusive); err != nil {
		_ = file.Close()
		return nil, err
	}
	err = unchanged()
	if err == nil {
		err = m.blockedByMarker()
	}
	if err != nil {
		_ = releaseLockFile(file)
		return nil, err
	}

	return file, nil
}

// releaseLockFile unlocks before closing, because a child process forked
// meanwhile shares the open file until it executes and would keep holding the
// lock after a plain close, and Windows may release the lock of a closed file
// only later.
func releaseLockFile(file *os.File) error {
	unlockErr := filelock.Unlock(file)

	return errors.Join(unlockErr, file.Close())
}

// openLockFile also returns a check that reports errDirectoryChanged once
// the lock file no longer belongs to the directory at m.path. Locking a file
// whose directory was renamed or removed after opening must not count.
func (m *DirectoryMutex) openLockFile(create bool) (*os.File, func() error, error) {
	flag := os.O_RDWR
	if create {
		flag |= os.O_CREATE
	}

	if m.keyedLockDir != "" {
		return m.openKeyedLockFile(flag)
	}

	path := filepath.Join(m.path, lockFileName)
	file, err := os.OpenFile(path, flag, lockFilePerm)
	if err != nil {
		return nil, nil, err
	}
	unchanged := func() error {
		opened, err := file.Stat()
		if err != nil {
			return err
		}
		current, err := os.Stat(path)
		if errors.Is(err, os.ErrNotExist) || (err == nil && !os.SameFile(opened, current)) {
			return errDirectoryChanged
		}

		return err
	}

	return file, unchanged, nil
}

func (m *DirectoryMutex) openKeyedLockFile(flag int) (*os.File, func() error, error) {
	dirID, err := directoryID(m.path)
	if err != nil {
		return nil, nil, err
	}
	if flag&os.O_CREATE != 0 {
		if err := os.MkdirAll(m.keyedLockDir, lockDirPerm); err != nil {
			return nil, nil, err
		}
	}
	file, err := os.OpenFile(filepath.Join(m.keyedLockDir, dirID+".lock"), flag, lockFilePerm)
	if err != nil {
		return nil, nil, err
	}
	unchanged := func() error {
		current, err := directoryID(m.path)
		if errors.Is(err, os.ErrNotExist) || (err == nil && current != dirID) {
			return errDirectoryChanged
		}

		return err
	}

	return file, unchanged, nil
}

func (m *DirectoryMutex) blockedByMarker() error {
	marker, err := m.findMarker()
	if err != nil {
		return err
	}
	if marker != "" {
		return filelock.ErrWouldBlock
	}

	return nil
}

func (m *DirectoryMutex) findMarker() (string, error) {
	entries, err := os.ReadDir(m.path)
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		if !entry.IsDir() && isMarkerName(entry.Name()) {
			return filepath.Join(m.path, entry.Name()), nil
		}
	}

	return "", nil
}

// transientError returns err if a retry can succeed once another process
// closes a conflicting file handle, and nil otherwise.
func (m *DirectoryMutex) transientError(err error) error {
	if err != nil && m.isTransient(err) {
		return err
	}

	return nil
}

// wrapAcquireError reports a transient file error that persisted until the
// acquire deadline as itself, because the lock may never have been held.
func wrapAcquireError(err, transientErr error) error {
	if !errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if transientErr != nil {
		return transientErr
	}

	return errors.Join(ErrAcquireTimeout, err)
}

func withTimeoutIfMissing(
	ctx context.Context,
	timeout time.Duration,
) (context.Context, context.CancelFunc) {
	if ctx == nil {
		return context.WithTimeout(context.Background(), timeout)
	}
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		return ctx, func() { /* no-op: caller-provided ctx already owns its own cancellation */ }
	}

	return context.WithTimeout(ctx, timeout)
}

func (m *DirectoryMutex) withLock(
	ctx context.Context,
	arg any,
	function func(any) error,
	exclusive bool,
) (err error) {
	if err := m.acquire(ctx, exclusive); err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, m.Release())
	}()

	return function(arg)
}

func sleepWithContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
