//go:build sidecar_test_catalog

// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package sidecars

import _ "embed"

//go:embed catalog.yaml
var Catalog []byte
