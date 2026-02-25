// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package directorymutex

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalidMarker    = errors.New("invalid marker")
	ErrAcquireTimeout   = errors.New("lock acquire timed out")
	ErrLockStateChanged = errors.New("lock state changed")
	ErrUnlockTimeout    = errors.New("unlock timed out")
	ErrPathNotDirectory = errors.New("path is not a directory")
)

const (
	modeSharedString    = "shared"
	modeExclusiveString = "exclusive"
)

const (
	markerStem          = ".dirmutex"
	markerPrefix        = markerStem + "."
	exclusiveMarkerName = markerStem + ".exclusive"
	sharedMarkerPrefix  = markerStem + ".shared."
)

const (
	defaultAcquireTimeout = 1 * time.Second
	defaultUnlockTimeout  = 5 * time.Second
	defaultRetryInterval  = 200 * time.Millisecond
)

type lockMode int

const (
	modeShared lockMode = iota + 1
	modeExclusive
)

var errWouldBlock = errors.New("lock unavailable")

// Status reports the observed lock state in a directory.
type Status struct {
	Locked      bool
	Mode        string
	SharedCount int
	MarkerName  string
	MarkerPath  string
}

// DirectoryMutex manages cross-process directory locks backed by marker files.
type DirectoryMutex struct {
	path string
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

	return &DirectoryMutex{path: path}, nil
}

// AcquireShared acquires a shared lock.
func (m *DirectoryMutex) AcquireShared(ctx context.Context) error {
	return m.acquire(ctx, modeShared)
}

// AcquireExclusive acquires an exclusive lock.
func (m *DirectoryMutex) AcquireExclusive(ctx context.Context) error {
	return m.acquire(ctx, modeExclusive)
}

// ReleaseShared releases a shared lock.
func (m *DirectoryMutex) ReleaseShared(ctx context.Context) error {
	return m.release(ctx, modeShared)
}

// ReleaseExclusive releases an exclusive lock.
func (m *DirectoryMutex) ReleaseExclusive(ctx context.Context) error {
	return m.release(ctx, modeExclusive)
}

// Status reports the current observed lock state.
func (m *DirectoryMutex) Status() (Status, error) {
	marker, err := m.findMarker()
	if err != nil {
		return Status{}, err
	}
	if marker == nil {
		return Status{Locked: false}, nil
	}

	status := Status{
		Locked:      true,
		SharedCount: marker.count,
		MarkerName:  marker.name,
		MarkerPath:  marker.path,
	}

	switch marker.mode {
	case modeShared:
		status.Mode = modeSharedString
	case modeExclusive:
		status.Mode = modeExclusiveString
	default:
		return Status{}, errors.ErrUnsupported
	}

	return status, nil
}

// ClearLock force-removes the current marker file if one exists.
func (m *DirectoryMutex) ClearLock() error {
	marker, err := m.findMarker()
	if err != nil {
		return err
	}
	if marker == nil {
		return nil
	}
	if err := os.Remove(marker.path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return err
	}

	return nil
}

type markerInfo struct {
	name  string
	path  string
	mode  lockMode
	count int
}

func (m *DirectoryMutex) acquire(ctx context.Context, mode lockMode) error {
	lockCtx, cancel := withTimeoutIfMissing(ctx, defaultAcquireTimeout)
	defer cancel()

	for {
		if err := lockCtx.Err(); err != nil {
			return wrapAcquireError(err)
		}

		locked, err := m.tryAcquire(mode)
		if err == nil && locked {
			return nil
		}
		if err != nil && !errors.Is(err, errWouldBlock) {
			return err
		}

		if err := sleepWithContext(lockCtx, defaultRetryInterval); err != nil {
			return wrapAcquireError(err)
		}
	}
}

func (m *DirectoryMutex) release(ctx context.Context, mode lockMode) error {
	unlockCtx, cancel := withTimeoutIfMissing(ctx, defaultUnlockTimeout)
	defer cancel()

	for {
		unlocked, err := m.tryRelease(mode)
		if err == nil && unlocked {
			return nil
		}
		if err != nil && !errors.Is(err, errWouldBlock) {
			return err
		}

		if err := sleepWithContext(unlockCtx, defaultRetryInterval); err != nil {
			return errors.Join(ErrUnlockTimeout, err)
		}
	}
}

func (m *DirectoryMutex) tryAcquire(mode lockMode) (bool, error) {
	marker, err := m.findMarker()
	if err != nil {
		return false, err
	}

	switch mode {
	case modeExclusive:
		if marker != nil {
			return false, errWouldBlock
		}

		return m.createMarker(exclusiveMarkerName)
	case modeShared:
		if marker == nil {
			return m.createMarker(sharedMarkerName(1))
		}
		if marker.mode == modeExclusive {
			return false, errWouldBlock
		}
		if marker.mode != modeShared {
			return false, ErrInvalidMarker
		}

		nextName := sharedMarkerName(marker.count + 1)
		nextPath := filepath.Join(m.path, nextName)
		if err := os.Rename(marker.path, nextPath); err != nil {
			if os.IsNotExist(err) || os.IsExist(err) {
				return false, errWouldBlock
			}

			return false, err
		}

		return true, nil
	default:
		return false, fmt.Errorf("unsupported lock mode: %d", mode)
	}
}

func (m *DirectoryMutex) tryRelease(mode lockMode) (bool, error) {
	marker, err := m.findMarker()
	if err != nil {
		return false, err
	}
	if marker == nil {
		return true, nil
	}

	switch mode {
	case modeExclusive:
		if marker.mode != modeExclusive {
			return false, ErrLockStateChanged
		}
		if err := os.Remove(marker.path); err != nil {
			if os.IsNotExist(err) {
				return false, errWouldBlock
			}

			return false, err
		}

		return true, nil
	case modeShared:
		if marker.mode != modeShared {
			return false, ErrLockStateChanged
		}

		if marker.count == 1 {
			if err := os.Remove(marker.path); err != nil {
				if os.IsNotExist(err) {
					return false, errWouldBlock
				}

				return false, err
			}

			return true, nil
		}

		nextName := sharedMarkerName(marker.count - 1)
		nextPath := filepath.Join(m.path, nextName)
		if err := os.Rename(marker.path, nextPath); err != nil {
			if os.IsNotExist(err) || os.IsExist(err) {
				return false, errWouldBlock
			}

			return false, err
		}

		return true, nil
	default:
		return false, fmt.Errorf("unsupported lock mode: %d", mode)
	}
}

func (m *DirectoryMutex) createMarker(name string) (bool, error) {
	path := filepath.Join(m.path, name)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) // nolint:mnd
	if err == nil {
		_ = file.Close()
		return true, nil
	}
	if os.IsExist(err) {
		return false, errWouldBlock
	}

	return false, err
}

func (m *DirectoryMutex) findMarker() (*markerInfo, error) {
	entries, err := os.ReadDir(m.path)
	if err != nil {
		return nil, err
	}

	var match *markerInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasPrefix(name, markerPrefix) {
			continue
		}

		marker, parseErr := parseMarker(name, m.path)
		if parseErr != nil {
			return nil, parseErr
		}
		if match != nil {
			return nil, ErrInvalidMarker
		}

		match = marker
	}

	return match, nil
}

func parseMarker(name string, directory string) (*markerInfo, error) {
	if name == exclusiveMarkerName {
		return &markerInfo{
			name: name,
			path: filepath.Join(directory, name),
			mode: modeExclusive,
		}, nil
	}

	if !strings.HasPrefix(name, sharedMarkerPrefix) {
		return nil, ErrInvalidMarker
	}

	countToken := strings.TrimPrefix(name, sharedMarkerPrefix)
	if countToken == "" {
		return nil, ErrInvalidMarker
	}
	for _, character := range countToken {
		if character < '0' || character > '9' {
			return nil, ErrInvalidMarker
		}
	}

	count, err := strconv.Atoi(countToken)
	if err != nil || count <= 0 {
		return nil, ErrInvalidMarker
	}

	return &markerInfo{
		name:  name,
		path:  filepath.Join(directory, name),
		mode:  modeShared,
		count: count,
	}, nil
}

func sharedMarkerName(count int) string {
	return sharedMarkerPrefix + strconv.Itoa(count)
}

func wrapAcquireError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return errors.Join(ErrAcquireTimeout, err)
	}

	return err
}

func withTimeoutIfMissing(
	ctx context.Context,
	timeout time.Duration,
) (context.Context, context.CancelFunc) {
	if ctx == nil {
		return context.WithTimeout(context.Background(), timeout)
	}
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		return ctx, func() {}
	}

	return context.WithTimeout(ctx, timeout)
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
