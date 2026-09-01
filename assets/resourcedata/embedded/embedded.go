// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package embedded

//go:generate go run ../../../tools/resourceembedder

import (
	"embed"
	"io/fs"
)

const resolvedSpecName = "resolved.yaml"

var Blobs fs.FS = embed.FS{}

var ResolvedSpec []byte
