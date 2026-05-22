// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

//go:build darwin && arm64

package localruntimebin

import _ "embed"

//go:embed generated/darwin/arm64/mac-runner-aarch64
var RunnerBinary []byte

const (
	RunnerBinaryName      = "mac-runner-aarch64"
	RunnerBinaryAvailable = true
)
