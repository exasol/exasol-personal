// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package presets

import "strings"

// PresetRef selects a preset either by embedded preset name or by a filesystem path.
// Exactly one of Name or Path should be set.
//
// Name refers to an embedded preset.
// Path refers to a directory on disk containing a preset manifest (e.g. infrastructure.yaml).
type PresetRef struct {
	Name string
	Path string
}

func (p PresetRef) IsPath() bool {
	return strings.TrimSpace(p.Path) != ""
}
