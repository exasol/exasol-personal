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

func validateLocalHostPlatform() error {
	if localRuntimePlatformSupported() {
		return nil
	}

	const unsupportedMessagePrefix = "local deployment is only supported on Apple Silicon macOS "
	const unsupportedMessageSuffix = "(darwin/arm64); current host is %s/%s"

	return fmt.Errorf(
		unsupportedMessagePrefix+unsupportedMessageSuffix,
		runtime.GOOS,
		runtime.GOARCH,
	)
}
