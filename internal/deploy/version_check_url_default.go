// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

//go:build !official_release

package deploy

// DefaultVersionCheckURL is the default endpoint used for version checking.
//
// For official release builds, this value is overridden via a build tag in
// version_check_url_official_release.go.
const DefaultVersionCheckURL = "https://metrics-test.exasol.com/v1/version-check"
