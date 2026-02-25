// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"strings"

	"github.com/exasol/exasol-personal/internal/presets"
)

func normalizePresetTypeFilter(typeFilter string) string {
	filter := strings.ToLower(strings.TrimSpace(typeFilter))
	if filter == "all" {
		return ""
	}
	// Keep CLI aliases stable.
	switch filter {
	case "infra":
		return presets.PresetTypeInfrastructure
	case "install":
		return presets.PresetTypeInstallation
	default:
		return filter
	}
}
