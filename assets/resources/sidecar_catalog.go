//go:build !sidecar_test_catalog

// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package resources

import _ "embed"

//go:embed sidecar-catalog.yaml
var SidecarCatalogYAML []byte
