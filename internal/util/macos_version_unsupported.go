// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

//go:build !darwin

package util

import (
	"context"
	"fmt"
	"runtime"
)

// GetMacOSMajorVersion returns the host's macOS product major version.
//
// Only macOS is supported; other platforms return an error since this is
// only meaningful for macOS-specific local deployment presets.
func GetMacOSMajorVersion(_ context.Context) (int, error) {
	return 0, fmt.Errorf("macOS version detection is not implemented for %s", runtime.GOOS)
}
