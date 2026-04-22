// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

//go:build windows

package localruntime

func IsProcessRunning(int) bool {
	return false
}
