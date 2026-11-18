// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

//go:build windows

package filelock

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// Windows locks are mandatory byte ranges, so the lock covers the largest
// range and lock files carry no content to read or write.
const allBytes = ^uint32(0)

func lockHandle(handle uintptr, exclusive bool) error {
	flags := uint32(windows.LOCKFILE_FAIL_IMMEDIATELY)
	if exclusive {
		flags |= windows.LOCKFILE_EXCLUSIVE_LOCK
	}

	err := windows.LockFileEx(
		windows.Handle(handle), flags, 0, allBytes, allBytes, new(windows.Overlapped),
	)
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return ErrWouldBlock
	} else if err != nil {
		return os.NewSyscallError("LockFileEx", err)
	}

	return nil
}

func unlockHandle(handle uintptr) error {
	err := windows.UnlockFileEx(
		windows.Handle(handle), 0, allBytes, allBytes, new(windows.Overlapped),
	)
	if err != nil {
		return os.NewSyscallError("UnlockFileEx", err)
	}

	return nil
}
