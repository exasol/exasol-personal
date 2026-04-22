// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"fmt"
	"runtime"
)

var localRuntimePlatformSupported = func() bool {
	return runtime.GOOS == "darwin" && runtime.GOARCH == "arm64"
}

func validateBackendPlatform(backendKind string) error {
	if backendKind != backendTypeLocal {
		return nil
	}
	if localRuntimePlatformSupported() {
		return nil
	}

	return fmt.Errorf(
		"local deployment is only supported on Apple Silicon macOS (darwin/arm64); current host is %s/%s",
		runtime.GOOS,
		runtime.GOARCH,
	)
}
