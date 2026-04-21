// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

//go:build linux && amd64

package tofubin

import _ "embed"

//go:embed generated/linux/amd64/tofu.tar.gz
var TofuArchive []byte

const TofuBinaryName = "tofu"
