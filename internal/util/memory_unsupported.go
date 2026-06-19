// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

//go:build !darwin

package util

import (
	"context"
	"fmt"
	"runtime"
)

// GetTotalMemoryMB returns the total physical host memory in megabytes.
//
// Only macOS is supported today. Other platforms return an error until host
// memory detection is implemented for them.
func GetTotalMemoryMB(_ context.Context) (uint64, error) {
	// Linux (/proc/meminfo) and Windows detection are not implemented yet;
	// local deployments only target macOS today.
	return 0, fmt.Errorf("total memory detection is not implemented for %s", runtime.GOOS)
}
