// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

// Package filelock locks open files with advisory operating system locks.
// A lock belongs to the open file, so two handles on the same file conflict
// even within one process, and the operating system releases it when the file
// is closed or the process exits.
package filelock

import (
	"errors"
	"os"
)

// ErrWouldBlock reports that another open file holds a conflicting lock.
var ErrWouldBlock = errors.New("file is locked by another handle")

// TryLock locks an unlocked file without waiting and returns ErrWouldBlock if
// another handle holds a conflicting lock.
func TryLock(file *os.File, exclusive bool) error {
	return control(file, func(handle uintptr) error {
		return lockHandle(handle, exclusive)
	})
}

// Unlock releases the lock that file holds.
func Unlock(file *os.File) error {
	return control(file, unlockHandle)
}

func control(file *os.File, operation func(handle uintptr) error) error {
	conn, err := file.SyscallConn()
	if err != nil {
		return err
	}

	var operationErr error
	if err := conn.Control(func(handle uintptr) {
		operationErr = operation(handle)
	}); err != nil {
		return err
	}

	return operationErr
}
