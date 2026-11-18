// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

//go:build unix

package directorymutex

import (
	"fmt"
	"os"
	"syscall"
)

// Directories hold their own lock file, which moves with a renamed directory.
func defaultKeyedLockDir() (string, error) {
	return "", nil
}

func isTransientFileError(error) bool {
	return false
}

// directoryID identifies path by device and inode, which survive a rename
// within the file system.
func directoryID(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", fmt.Errorf("identify %s: unsupported file information", path)
	}

	return fmt.Sprintf("%x-%x", stat.Dev, stat.Ino), nil
}
