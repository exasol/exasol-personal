// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

//go:build windows && amd64

package embedded

import (
	"embed"
	"io/fs"
)

// all: includes the tracked .gitignore, keeping empty output embeddable.
//
//go:embed all:data/windows_amd64
var platform embed.FS

// //go:embed requires a package variable, so derived views are initialized here.
//
//nolint:gochecknoinits
func init() {
	sub, err := fs.Sub(platform, "data/windows_amd64")
	if err != nil {
		panic(err)
	}
	Blobs = sub
	ResolvedSpec, _ = fs.ReadFile(sub, resolvedSpecName)
}
