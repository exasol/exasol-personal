// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

//go:build windows

package directorymutex

import (
	"errors"
	"fmt"
	"os"

	"github.com/exasol/exasol-personal/internal/launcherpaths"
	"golang.org/x/sys/windows"
)

// Windows cannot rename a directory while a file inside it is open, and
// migration renames locked directories, so their lock files live elsewhere.
func defaultKeyedLockDir() (string, error) {
	return launcherpaths.LocksRootPath()
}

// Windows refuses to open a file or directory while another process briefly
// holds a conflicting handle, for example a concurrent launcher renaming or
// removing the directory, or a virus scanner. Such failures clear once that
// handle closes.
func isTransientFileError(err error) bool {
	return errors.Is(err, windows.ERROR_SHARING_VIOLATION) ||
		errors.Is(err, windows.ERROR_ACCESS_DENIED) ||
		errors.Is(err, windows.ERROR_LOCK_VIOLATION)
}

// directoryID identifies path by volume serial number and file index, which
// survive a rename within the volume.
func directoryID(path string) (string, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	handle, err := windows.CreateFile(
		name,
		windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS,
		0,
	)
	if err != nil {
		return "", &os.PathError{Op: "open", Path: path, Err: err}
	}
	defer func() { _ = windows.CloseHandle(handle) }()

	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		return "", &os.PathError{Op: "identify", Path: path, Err: err}
	}

	return fmt.Sprintf(
		"%08x-%08x%08x", info.VolumeSerialNumber, info.FileIndexHigh, info.FileIndexLow,
	), nil
}
