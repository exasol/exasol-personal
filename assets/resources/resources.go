// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package resources

import _ "embed"

// ResourcesYAML contains the embedded runtime resource specification.
//
//go:embed resources.yaml
var ResourcesYAML []byte
