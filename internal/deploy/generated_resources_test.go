// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

// Production binaries register embedded resource data by importing this
// package for its side effect (see cmd/exasol/main.go); tests need the same
// import to resolve real named presets (e.g. "aws", "local") through the
// resource cache instead of seeing an empty catalog.
import _ "github.com/exasol/exasol-personal/assets/resources/generated"
