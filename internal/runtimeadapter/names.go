// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package runtimeadapter

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

const kubeNameMaxLength = 63

var nonDNSLabel = regexp.MustCompile(`[^a-z0-9-]+`)

func WorkloadName(deploymentID string) string {
	normalized := strings.ToLower(strings.TrimSpace(deploymentID))
	normalized = nonDNSLabel.ReplaceAllString(normalized, "-")
	normalized = strings.Trim(normalized, "-")
	prefix := "exasol-personal-"
	if normalized == "" {
		sum := sha256.Sum256([]byte(deploymentID))
		normalized = hex.EncodeToString(sum[:6])
	}
	if len(prefix)+len(normalized) <= kubeNameMaxLength {
		return prefix + normalized
	}
	sum := sha256.Sum256([]byte(deploymentID))
	suffix := fmt.Sprintf("-%x", sum[:6])
	available := kubeNameMaxLength - len(prefix) - len(suffix)

	return prefix + strings.TrimRight(normalized[:available], "-") + suffix
}
