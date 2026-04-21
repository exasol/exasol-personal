// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

//go:build windows && amd64

package tofubin

import _ "embed"

//go:embed generated/windows/amd64/tofu.tar.gz
var TofuArchive []byte

const TofuBinaryName = "tofu.exe"
