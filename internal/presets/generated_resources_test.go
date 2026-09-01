// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package presets

// Production binaries register embedded resource data by importing this
// package for its side effect (see cmd/exasol/main.go); tests need the same
// import to resolve real embedded resources through the resource cache
// instead of seeing an empty catalog.
import _ "github.com/exasol/exasol-personal/assets/resourcedata/generated"
