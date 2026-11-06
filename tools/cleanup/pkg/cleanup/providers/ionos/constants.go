// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package ionos

import "regexp"

// ProviderName is the identifier for the IONOS Cloud provider.
const ProviderName = "ionos"

// DefaultLocation is the sentinel location meaning "all locations in the
// contract". The IONOS Cloud API lists datacenters and IP blocks across every
// location in one call, so a location is a client-side filter.
const DefaultLocation = "all"

const pageSize = int32(100)

// IONOS resources carry no deployment labels except on servers, so the launcher
// naming scheme identifies them (see assets/infrastructure/ionos/main.tf): the
// datacenter it creates is named exactly after the deployment, and every other
// resource is prefixed with the deployment id.
var (
	deploymentIDRegex   = regexp.MustCompile(`^exasol-[a-f0-9]{8}$`)
	resourceNamePattern = regexp.MustCompile(`^(exasol-[a-f0-9]{8})-.+$`)
)
