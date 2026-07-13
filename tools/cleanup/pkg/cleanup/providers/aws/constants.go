// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

//nolint:gci,gofumpt // formatting of small constants block is acceptable and consistent.
package aws

import "time"

// Lint-friendly shared constants.
const (
	arnSplitParts     = 6
	minSegs           = 2
	resourcesPerPage  = int32(100)
	s3BatchDeleteSize = 1000
	igwDeleteRetries  = 3
)

var igwDeleteRetryDelay = 2 * time.Second
