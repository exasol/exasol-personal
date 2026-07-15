// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package resources

import _ "embed"

// SLCCatalogYAML contains the embedded official script language container catalog
// (official SLCs for exasol personal local deployments installed locally via Podman image mount).
//
//go:embed slc-catalog.yaml
var SLCCatalogYAML []byte
