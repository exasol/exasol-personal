// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package tofu

import (
	"os"
	"path/filepath"

	"github.com/exasol/exasol-personal/assets/tofubin"
)

// BinaryName is the platform-specific tofu binary name.
const BinaryName = tofubin.TofuBinaryName

// WriteBinary writes the embedded tofu binary to the given path.
func WriteBinary(path string) error {
	const perm = 0o744
	return os.WriteFile(path, tofubin.TofuBinary, perm)
}

// AlpineImageName is the platform-specific alpine image name.
const AlpineImageName = tofubin.AlpineImageName

// WriteVMData writes the embedded alpine image to the given path.
func WriteVMData(path string) (err error) {
	const perm = 0o744
	err = os.WriteFile(filepath.Join(path, tofubin.AlpineImageName), tofubin.AlpineImage, perm)
	if err != nil {
		return
	}

	err = os.WriteFile(filepath.Join(path, tofubin.CloudInitImageName), tofubin.CloudInitImage, perm)
	if err != nil {
		return
	}

	err = os.WriteFile(filepath.Join(path, tofubin.VmSshKeyName), tofubin.VmSshKey, perm)
	if err != nil {
		return
	}

	return
}
