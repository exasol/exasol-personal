// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package vm

import (
	"fmt"
	"path/filepath"
	"strings"
)

const mib = 1024 * 1024

func clampCPUCount(requested int, min, max uint) uint {
	switch {
	case requested <= 0:
		return min
	case uint(requested) < min:
		return min
	case uint(requested) > max:
		return max
	default:
		return uint(requested)
	}
}

func clampMemoryBytes(requested, min, max uint64) uint64 {
	value := requested
	if value < min {
		value = min
	}
	if value > max {
		value = max
	}
	if value%mib != 0 {
		value -= value % mib
	}
	if value < min {
		return min
	}
	return value
}

func resolvedSharedDirTag(sharedDir SharedDir, index int) string {
	if tag := sanitizeTag(sharedDir.Tag); tag != "" {
		return tag
	}

	base := strings.Trim(strings.TrimSpace(sharedDir.Destination), "/")
	if base == "" {
		base = filepath.Base(strings.TrimSpace(sharedDir.Source))
	}
	if base == "" || base == "." || base == string(filepath.Separator) {
		return fmt.Sprintf("share-%d", index+1)
	}

	if tag := sanitizeTag(base); tag != "" {
		return tag
	}

	return fmt.Sprintf("share-%d", index+1)
}

func sanitizeTag(value string) string {
	var builder strings.Builder
	lastDash := false

	for _, r := range strings.TrimSpace(value) {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			builder.WriteRune(r)
			lastDash = false
		default:
			if lastDash || builder.Len() == 0 {
				continue
			}
			builder.WriteByte('-')
			lastDash = true
		}
	}

	return strings.Trim(builder.String(), "-")
}
