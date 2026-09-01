// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

//go:build darwin && arm64

package embedded

import (
	"embed"
	"io/fs"
)

// all: includes the tracked .gitignore, keeping empty output embeddable.
//
//go:embed all:data/darwin_arm64
var platform embed.FS

// //go:embed requires a package variable, so derived views are initialized here.
//
//nolint:gochecknoinits
func init() {
	sub, err := fs.Sub(platform, "data/darwin_arm64")
	if err != nil {
		panic(err)
	}
	Blobs = sub
	ResolvedSpec, _ = fs.ReadFile(sub, resolvedSpecName)
}
