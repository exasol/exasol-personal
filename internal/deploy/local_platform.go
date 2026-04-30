// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

const minimumLocalRuntimeMacOSMajor = 13

var (
	localRuntimePlatformSupported = func() bool {
		if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
			return false
		}
		major, err := readMacOSMajorVersion()
		if err != nil {
			return false
		}

		return major >= minimumLocalRuntimeMacOSMajor
	}

	macOSProductVersionLookup = lookupMacOSProductVersion
)

func validateLocalHostPlatform() error {
	if localRuntimePlatformSupported() {
		return nil
	}

	return errors.New(
		"local deployment requires Apple Silicon macOS 13 (Ventura) or later " +
			"because EFI VM boot is unavailable on earlier versions",
	)
}

func readMacOSMajorVersion() (int, error) {
	version, err := macOSProductVersionLookup()
	if err != nil {
		return 0, err
	}
	majorPart, _, _ := strings.Cut(strings.TrimSpace(version), ".")
	major, err := strconv.Atoi(majorPart)
	if err != nil {
		return 0, fmt.Errorf("failed to parse macOS major version %q: %w", version, err)
	}

	return major, nil
}

func lookupMacOSProductVersion() (string, error) {
	out, err := exec.CommandContext(
		context.Background(),
		"sw_vers",
		"-productVersion",
	).Output()
	if err != nil {
		return "", fmt.Errorf("failed to read macOS product version: %w", err)
	}

	return strings.TrimSpace(string(out)), nil
}
