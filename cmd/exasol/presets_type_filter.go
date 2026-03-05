// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"strings"

	"github.com/exasol/exasol-personal/internal/presets"
)

const (
	filterTypeInfra   = "infra"
	filterTypeInstall = "install"
)

func normalizePresetTypeFilter(typeFilter string) string {
	filter := strings.ToLower(strings.TrimSpace(typeFilter))
	if filter == "all" {
		return ""
	}
	// Keep CLI aliases stable.
	switch filter {
	case filterTypeInfra:
		return presets.PresetTypeInfrastructure
	case filterTypeInstall:
		return presets.PresetTypeInstallation
	default:
		return filter
	}
}
