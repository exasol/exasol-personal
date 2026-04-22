// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

//go:build !windows

package localruntime

import (
	"errors"
	"syscall"
)

func IsProcessRunning(pid int) bool {
	err := syscall.Kill(pid, syscall.Signal(0))

	return err == nil || errors.Is(err, syscall.EPERM)
}
