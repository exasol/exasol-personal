// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

//go:build darwin && arm64

package tofubin

import _ "embed"

//go:embed generated/darwin/arm64/tofu.tar.gz
var TofuArchive []byte

const TofuBinaryName = "tofu"
