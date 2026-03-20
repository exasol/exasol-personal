// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package exoscale

import "time"

// Exoscale API constants
const (
	defaultPageSize = int64(100)
	defaultZone     = "ch-gva-2"
)

var defaultTimeout = 30 * time.Second
