// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package tofu

import (
	"os"

	"github.com/exasol/exasol-personal/assets/tofubin"
)

// BinaryName is the platform-specific tofu binary name.
const BinaryName = tofubin.TofuBinaryName

// WriteBinary writes the embedded tofu binary to the given path.
func WriteBinary(path string) error {
	const perm = 0o744
	return os.WriteFile(path, tofubin.TofuBinary, perm)
}
