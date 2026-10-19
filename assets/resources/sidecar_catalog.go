// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package resources

import _ "embed" // required for the go:embed directive below

//go:embed sidecar-catalog.yaml
var SidecarCatalogYAML []byte
