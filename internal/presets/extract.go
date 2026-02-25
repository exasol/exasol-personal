// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package presets

import "github.com/exasol/exasol-personal/internal/util"

// ExtractPreset writes a preset into dstDir.
//
// If preset.Path is set, it copies the directory from disk.
// Otherwise it uses writeEmbedded to materialize the embedded preset named preset.Name.
func ExtractPreset(
	preset PresetRef,
	dstDir string,
	writeEmbedded func(name string, outDir string) error,
) error {
	if preset.IsPath() {
		return util.CopyDir(preset.Path, dstDir)
	}

	return writeEmbedded(preset.Name, dstDir)
}
