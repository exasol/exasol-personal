// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

//go:build linux && arm64

package tofubin

import _ "embed"

//go:embed generated/linux/arm64/tofu
var TofuBinary []byte

const TofuBinaryName = "tofu"
