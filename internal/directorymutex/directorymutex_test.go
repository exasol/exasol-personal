// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package directorymutex

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestNewRejectsNonDirectory(t *testing.T) {
	t.Parallel()

	// Given a file path (not a directory)
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil { // nolint:mnd
		t.Fatalf("write file: %v", err)
	}

	// When creating a DirectoryMutex with that path
	_, err := New(file)

	// Then ErrPathNotDirectory is returned
	if !errors.Is(err, ErrPathNotDirectory) {
		t.Fatalf("expected ErrPathNotDirectory, got %v", err)
	}
}

func TestIsMarkerNameRecognizesDirectoryMutexMarkers(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name  string
		input string
		match bool
	}{
		{name: "exclusive", input: exclusiveMarkerName, match: true},
		{name: "shared", input: sharedMarkerName(3), match: true},
		{name: "non-marker", input: "notes.txt", match: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := IsMarkerName(test.input); got != test.match {
				t.Fatalf("expected %q match=%t, got %t", test.input, test.match, got)
			}
		})
	}
}

func TestExclusiveLockBlocksSharedUntilTimeout(t *testing.T) {
	t.Parallel()

	// Given an exclusive lock is held
	dir := t.TempDir()
	mutex, err := New(dir)
	if err != nil {
		t.Fatalf("new mutex: %v", err)
	}

	exclusiveAcquired := make(chan struct{})
	releaseExclusive := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- holdExclusive(mutex, exclusiveAcquired, releaseExclusive)
	}()
	<-exclusiveAcquired

	// When trying to acquire a shared lock with a short deadline
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err = mutex.AcquireShared(ctx)

	// Then the acquisition times out
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context deadline exceeded, got %v", err)
	}
	if !errors.Is(err, ErrAcquireTimeout) {
		t.Fatalf("expected ErrAcquireTimeout, got %v", err)
	}

	close(releaseExclusive)
	if err := <-done; err != nil {
		t.Fatalf("exclusive lock goroutine failed: %v", err)
	}
}

func TestSharedLockCountsAndRelease(t *testing.T) {
	t.Parallel()

	// Given two shared lock holders on the same directory
	dir := t.TempDir()
	mutexA, err := New(dir)
	if err != nil {
		t.Fatalf("new mutex A: %v", err)
	}
	mutexB, err := New(dir)
	if err != nil {
		t.Fatalf("new mutex B: %v", err)
	}

	acquiredA := make(chan struct{})
	acquiredB := make(chan struct{})
	releaseA := make(chan struct{})
	releaseB := make(chan struct{})
	doneA := make(chan error, 1)
	doneB := make(chan error, 1)

	go func() {
		doneA <- holdShared(mutexA, acquiredA, releaseA)
	}()
	go func() {
		doneB <- holdShared(mutexB, acquiredB, releaseB)
	}()

	<-acquiredA
	<-acquiredB

	// When both shared locks are active
	status, err := mutexA.Status()
	if err != nil {
		t.Fatalf("status: %v", err)
	}

	// Then count is 2 and mode is shared
	if !status.Locked {
		t.Fatalf("expected locked status, got %+v", status)
	}
	if status.Mode != modeSharedString {
		t.Fatalf("expected mode %q, got %q", modeSharedString, status.Mode)
	}
	if status.SharedCount != 2 {
		t.Fatalf("expected shared count 2, got %d", status.SharedCount)
	}

	// When one holder releases
	close(releaseB)
	if err := <-doneB; err != nil {
		t.Fatalf("shared lock B failed: %v", err)
	}

	// Then count is decremented
	status, err = mutexA.Status()
	if err != nil {
		t.Fatalf("status after first release: %v", err)
	}
	if status.SharedCount != 1 {
		t.Fatalf("expected shared count 1, got %d", status.SharedCount)
	}

	// When the last holder releases
	close(releaseA)
	if err := <-doneA; err != nil {
		t.Fatalf("shared lock A failed: %v", err)
	}

	// Then lock state is unlocked
	status, err = mutexA.Status()
	if err != nil {
		t.Fatalf("status after second release: %v", err)
	}
	if status.Locked {
		t.Fatalf("expected unlocked status, got %+v", status)
	}
}

func TestSharedAcquireTimeoutLeavesExclusiveMarkerUntouched(t *testing.T) {
	t.Parallel()

	// Given an exclusive lock is held
	dir := t.TempDir()
	mutex, err := New(dir)
	if err != nil {
		t.Fatalf("new mutex: %v", err)
	}

	exclusiveAcquired := make(chan struct{})
	releaseExclusive := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- holdExclusive(mutex, exclusiveAcquired, releaseExclusive)
	}()
	<-exclusiveAcquired

	// When shared acquisition times out
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()
	err = mutex.AcquireShared(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context deadline exceeded, got %v", err)
	}

	// Then the lock marker still reports exclusive
	status, err := mutex.Status()
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.Mode != modeExclusiveString {
		t.Fatalf("expected mode %q, got %q", modeExclusiveString, status.Mode)
	}

	close(releaseExclusive)
	if err := <-done; err != nil {
		t.Fatalf("exclusive lock goroutine failed: %v", err)
	}
}

func TestInvalidMarkerFailsStatus(t *testing.T) {
	t.Parallel()

	// Given an invalid marker file
	dir := t.TempDir()
	mutex, err := New(dir)
	if err != nil {
		t.Fatalf("new mutex: %v", err)
	}

	invalidMarker := filepath.Join(dir, markerStem+".broken")
	if err := os.WriteFile(invalidMarker, nil, 0o600); err != nil { // nolint:mnd
		t.Fatalf("write invalid marker: %v", err)
	}

	// When reading status
	_, err = mutex.Status()

	// Then the invalid marker is rejected
	if !errors.Is(err, ErrInvalidMarker) {
		t.Fatalf("expected ErrInvalidMarker, got %v", err)
	}
}

func TestClearLockRemovesMarker(t *testing.T) {
	t.Parallel()

	// Given an exclusive lock is held
	dir := t.TempDir()
	mutex, err := New(dir)
	if err != nil {
		t.Fatalf("new mutex: %v", err)
	}

	exclusiveAcquired := make(chan struct{})
	releaseExclusive := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- holdExclusive(mutex, exclusiveAcquired, releaseExclusive)
	}()
	<-exclusiveAcquired

	// When force-clearing the lock
	if err := mutex.ClearLock(); err != nil {
		t.Fatalf("clear lock: %v", err)
	}

	// Then no marker remains
	status, err := mutex.Status()
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.Locked {
		t.Fatalf("expected unlocked status, got %+v", status)
	}

	close(releaseExclusive)
	if err := <-done; err != nil {
		t.Fatalf("exclusive lock goroutine failed: %v", err)
	}
}

// nolint: paralleltest
func TestSharedLockStressLeavesUnlocked(t *testing.T) {
	t.Skip("Flaky. With 'invalid marker' error. Somebody fix it")

	// Given many concurrent shared lock attempts
	dir := t.TempDir()
	mutex, err := New(dir)
	if err != nil {
		t.Fatalf("new mutex: %v", err)
	}

	const (
		workers        = 4
		iterations     = 20
		acquireTimeout = 3 * time.Second
	)

	// When all workers repeatedly acquire and release shared locks
	start := make(chan struct{})
	var waitGroup sync.WaitGroup
	errCh := make(chan error, workers)

	for range workers {
		waitGroup.Go(func() {
			<-start
			acquireReleaseSharedRepeatedly(mutex, iterations, acquireTimeout, errCh)
		})
	}
	close(start)
	waitGroup.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("shared lock stress failed: %v", err)
		}
	}

	// Then final status is unlocked
	status, err := mutex.Status()
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.Locked {
		t.Fatalf("expected unlocked status, got %+v", status)
	}
}

func acquireReleaseSharedRepeatedly(
	mutex *DirectoryMutex,
	iterations int,
	acquireTimeout time.Duration,
	errCh chan<- error,
) {
	for range iterations {
		ctx, cancel := context.WithTimeout(context.Background(), acquireTimeout)
		err := mutex.AcquireShared(ctx)
		cancel()
		if err != nil {
			errCh <- err
			return
		}
		if err := mutex.ReleaseShared(context.Background()); err != nil {
			errCh <- err
			return
		}
	}
}

func holdExclusive(mutex *DirectoryMutex, acquired chan<- struct{}, release <-chan struct{}) error {
	if err := mutex.AcquireExclusive(context.Background()); err != nil {
		return err
	}
	defer func() {
		_ = mutex.ReleaseExclusive(context.Background())
	}()

	close(acquired)
	<-release

	return nil
}

func holdShared(mutex *DirectoryMutex, acquired chan<- struct{}, release <-chan struct{}) error {
	if err := mutex.AcquireShared(context.Background()); err != nil {
		return err
	}
	defer func() {
		_ = mutex.ReleaseShared(context.Background())
	}()

	close(acquired)
	<-release

	return nil
}

func TestAcquireExclusiveIfUnlockedAcquiresOnlyAnUnlockedDirectory(t *testing.T) {
	t.Parallel()

	// Given one mutex holding a shared lock and another on an unlocked directory
	locked, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("new mutex: %v", err)
	}
	if err := locked.AcquireShared(context.Background()); err != nil {
		t.Fatalf("acquire shared: %v", err)
	}
	unlocked, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("new mutex: %v", err)
	}

	// When each tries to acquire an exclusive lock without waiting
	lockedAcquired, lockedErr := locked.AcquireExclusiveIfUnlocked()
	unlockedAcquired, unlockedErr := unlocked.AcquireExclusiveIfUnlocked()

	// Then only the unlocked directory is acquired and the shared lock is untouched
	if lockedErr != nil || lockedAcquired {
		t.Fatalf("locked directory: acquired=%t err=%v", lockedAcquired, lockedErr)
	}
	if unlockedErr != nil || !unlockedAcquired {
		t.Fatalf("unlocked directory: acquired=%t err=%v", unlockedAcquired, unlockedErr)
	}
	status, err := locked.Status()
	if err != nil || status.Mode != modeSharedString || status.SharedCount != 1 {
		t.Fatalf("expected one shared lock, got %+v err=%v", status, err)
	}
	status, err = unlocked.Status()
	if err != nil || status.Mode != modeExclusiveString {
		t.Fatalf("expected exclusive lock, got %+v err=%v", status, err)
	}
}

// newPermissionDeniedMutex returns a mutex on a read-only directory that treats
// the resulting permission errors like Windows' transient handle conflicts.
func newPermissionDeniedMutex(t *testing.T) (*DirectoryMutex, func()) {
	t.Helper()
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("relies on POSIX directory permissions enforced for a non-root user")
	}
	dir := t.TempDir()
	//nolint:gosec,mnd // A read-only directory makes marker updates fail.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	allow := func() {
		_ = os.Chmod(dir, 0o700) // nolint:gosec,mnd
	}
	t.Cleanup(allow)
	mutex, err := New(dir)
	if err != nil {
		t.Fatalf("new mutex: %v", err)
	}
	mutex.isTransient = func(err error) bool { return errors.Is(err, fs.ErrPermission) }

	return mutex, allow
}

func TestAcquireRetriesTransientFileErrorsUntilTheyClear(t *testing.T) {
	t.Parallel()

	// Given a directory that refuses marker creation for a moment
	mutex, allow := newPermissionDeniedMutex(t)
	time.AfterFunc(3*defaultRetryInterval, allow)

	// When acquiring with time to spare
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := mutex.AcquireExclusive(ctx)
	// Then the lock is acquired once the error clears
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
}

func TestAcquireReportsATransientFileErrorThatOutlastsTheTimeout(t *testing.T) {
	t.Parallel()

	// Given a directory that keeps refusing marker creation
	mutex, _ := newPermissionDeniedMutex(t)

	// When acquiring until the deadline
	ctx, cancel := context.WithTimeout(context.Background(), 3*defaultRetryInterval)
	defer cancel()
	err := mutex.AcquireExclusive(ctx)

	// Then the file error is reported instead of a held lock
	if !errors.Is(err, fs.ErrPermission) || errors.Is(err, ErrAcquireTimeout) {
		t.Fatalf("expected the permission error without ErrAcquireTimeout, got %v", err)
	}
}

func TestReleaseRetriesTransientFileErrorsUntilTheyClear(t *testing.T) {
	t.Parallel()

	// Given a held lock whose directory refuses marker removal for a moment
	mutex, allow := newPermissionDeniedMutex(t)
	allow()
	if err := mutex.AcquireExclusive(context.Background()); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	//nolint:gosec,mnd // A read-only directory makes marker removal fail.
	if err := os.Chmod(mutex.path, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	time.AfterFunc(3*defaultRetryInterval, allow)

	// When releasing
	err := mutex.ReleaseExclusive(context.Background())
	// Then the lock is released once the error clears
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	status, err := mutex.Status()
	if err != nil || status.Locked {
		t.Fatalf("expected unlocked, got %+v err=%v", status, err)
	}
}
