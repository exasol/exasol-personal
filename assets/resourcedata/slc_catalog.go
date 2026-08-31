// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package resourcedata

import _ "embed" // required for the go:embed directive below

//go:embed slc-catalog.yaml
var SLCCatalogYAML []byte
