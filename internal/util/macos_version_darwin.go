// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

//go:build darwin

package util

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// GetMacOSMajorVersion returns the host's macOS product major version (e.g.
// 15 for macOS Sequoia), read from the kern.osproductversion sysctl.
func GetMacOSMajorVersion(_ context.Context) (int, error) {
	version, err := unix.Sysctl("kern.osproductversion")
	if err != nil {
		return 0, fmt.Errorf("failed to read macOS product version: %w", err)
	}
	major, _, _ := strings.Cut(version, ".")
	majorVersion, err := strconv.Atoi(major)
	if err != nil {
		return 0, fmt.Errorf("failed to parse macOS product version %q: %w", version, err)
	}

	return majorVersion, nil
}
