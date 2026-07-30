// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

//go:build official_release

package version_check

// DefaultVersionCheckURL is the default endpoint used for version checking in
// official release builds.
const DefaultVersionCheckURL = "https://metrics.exasol.com/v1/version-check"
