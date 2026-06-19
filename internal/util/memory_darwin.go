// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

//go:build darwin

package util

import (
	"context"
	"fmt"

	"golang.org/x/sys/unix"
)

const bytesPerMegabyte = 1024 * 1024

// GetTotalMemoryMB returns the total physical host memory in megabytes.
func GetTotalMemoryMB(_ context.Context) (uint64, error) {
	value, err := unix.SysctlUint64("hw.memsize")
	if err != nil {
		return 0, fmt.Errorf("failed to read macOS host memory: %w", err)
	}

	return value / bytesPerMegabyte, nil
}
