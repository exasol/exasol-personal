// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

//go:build unix

package filelock

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func lockHandle(handle uintptr, exclusive bool) error {
	how := unix.LOCK_SH
	if exclusive {
		how = unix.LOCK_EX
	}

	return flock(handle, how|unix.LOCK_NB)
}

func unlockHandle(handle uintptr) error {
	return flock(handle, unix.LOCK_UN)
}

func flock(handle uintptr, how int) error {
	for {
		err := unix.Flock(int(handle), how) //nolint:gosec // File descriptors fit in int.
		switch {
		case err == nil:
			return nil
		case errors.Is(err, unix.EINTR):
			continue
		case errors.Is(err, unix.EWOULDBLOCK):
			return ErrWouldBlock
		default:
			return os.NewSyscallError("flock", err)
		}
	}
}
