// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package util

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

const bytesPerMegabyte = 1024 * 1024

// GetTotalMemoryMB returns the total physical host memory in megabytes.
//
// Only macOS is supported today. Other platforms return an error until host
// memory detection is implemented for them.
func GetTotalMemoryMB(ctx context.Context) (uint64, error) {
	switch runtime.GOOS {
	case "darwin":
		return darwinTotalMemoryMB(ctx)
	default:
		// Linux (/proc/meminfo) and Windows detection are not implemented yet;
		// local deployments only target macOS today.
		return 0, fmt.Errorf("total memory detection is not implemented for %s", runtime.GOOS)
	}
}

func darwinTotalMemoryMB(ctx context.Context) (uint64, error) {
	output, err := exec.CommandContext(ctx, "sysctl", "-n", "hw.memsize").Output()
	if err != nil {
		return 0, fmt.Errorf("failed to read macOS host memory: %w", err)
	}

	value, err := strconv.ParseUint(strings.TrimSpace(string(output)), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse macOS host memory: %w", err)
	}

	return value / bytesPerMegabyte, nil
}
