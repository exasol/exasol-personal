// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package assets

import (
	"embed"
)

//go:embed all:shared/**
var SharedAssets embed.FS

const SharedAssetDir = "shared"
