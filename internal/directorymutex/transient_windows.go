// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

//go:build windows

package directorymutex

import (
	"errors"

	"golang.org/x/sys/windows"
)

// Windows refuses to open, create, rename, or delete a file while another
// process briefly holds a conflicting handle, for example a concurrent
// launcher listing or removing the directory, or a virus scanner opening a
// new marker. Such failures clear once that handle closes.
func isTransientFileError(err error) bool {
	return errors.Is(err, windows.ERROR_SHARING_VIOLATION) ||
		errors.Is(err, windows.ERROR_ACCESS_DENIED) ||
		errors.Is(err, windows.ERROR_LOCK_VIOLATION)
}
