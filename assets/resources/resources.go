// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package resources

import _ "embed" // required for the go:embed directive below

// ResourcesYAML contains the embedded runtime resource specification.
//
//go:embed resources.yaml
var ResourcesYAML []byte
