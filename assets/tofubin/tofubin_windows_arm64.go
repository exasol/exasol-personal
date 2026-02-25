// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

//go:build windows && arm64

package tofubin

import _ "embed"

//go:embed generated/windows/arm64/tofu.exe
var TofuBinary []byte

const TofuBinaryName = "tofu.exe"
