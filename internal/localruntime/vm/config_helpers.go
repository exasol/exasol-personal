// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package vm

import (
	"fmt"
	"path/filepath"
	"strings"
)

const mib = 1024 * 1024

func clampCPUCount(requested int, minValue, maxValue uint) uint {
	switch {
	case requested <= 0:
		return minValue
	case uint(requested) < minValue:
		return minValue
	case uint(requested) > maxValue:
		return maxValue
	default:
		return uint(requested)
	}
}

func clampMemoryBytes(requested, minValue, maxValue uint64) uint64 {
	value := requested
	if value < minValue {
		value = minValue
	}
	if value > maxValue {
		value = maxValue
	}
	if value%mib != 0 {
		value -= value % mib
	}
	if value < minValue {
		return minValue
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

	for _, runeValue := range strings.TrimSpace(value) {
		switch {
		case runeValue >= 'a' && runeValue <= 'z',
			runeValue >= 'A' && runeValue <= 'Z',
			runeValue >= '0' && runeValue <= '9',
			runeValue == '.',
			runeValue == '_',
			runeValue == '-':
			_, _ = builder.WriteRune(runeValue)
			lastDash = false
		default:
			if lastDash || builder.Len() == 0 {
				continue
			}
			_ = builder.WriteByte('-')
			lastDash = true
		}
	}

	return strings.Trim(builder.String(), "-")
}
