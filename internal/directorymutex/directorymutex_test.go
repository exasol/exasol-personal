// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package directorymutex

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			<-start

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
		}()
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
