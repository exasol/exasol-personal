// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package aws

import (
	"os"

	"github.com/exasol/exasol-personal/tools/cleanup/internal/shared"
)

// ResolveRegion returns the explicit region if provided, otherwise falls back to
// AWS_REGION then AWS_DEFAULT_REGION. Returns error if neither is available.
func ResolveRegion(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if v := os.Getenv("AWS_REGION"); v != "" {
		return v, nil
	}
	if v := os.Getenv("AWS_DEFAULT_REGION"); v != "" {
		return v, nil
	}

	return "", shared.ErrRegionRequired
}
