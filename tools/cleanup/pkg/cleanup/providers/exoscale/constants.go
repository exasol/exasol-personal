// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package exoscale

import (
	"regexp"
	"time"
)

// Exoscale API constants
const (
	defaultPageSize = int64(100)
	defaultZone     = "ch-gva-2"
)

var defaultTimeout = 30 * time.Second

// resourceNamePattern matches names of resources derived from a deployment id.
var resourceNamePattern = regexp.MustCompile(`^(exasol-[a-f0-9]{8})-.+$`)
