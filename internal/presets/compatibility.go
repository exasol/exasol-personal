// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package presets

import (
	"slices"
	"strings"
)

// Compatibility declares directional preset-composition metadata.
//
// Infrastructure presets use Provides, installation presets use Requires.
type Compatibility struct {
	Provides []string `yaml:"provides,omitempty"`
	Requires []string `yaml:"requires,omitempty"`
}

func normalizedCapabilities(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))

	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}

		seen[value] = struct{}{}
		result = append(result, value)
	}

	slices.Sort(result)

	return result
}
