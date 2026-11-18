// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

//go:build !windows

package directorymutex

func isTransientFileError(error) bool {
	return false
}
