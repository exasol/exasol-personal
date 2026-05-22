// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

//go:build !darwin || !arm64

package localruntimebin

var RunnerBinary []byte

const (
	RunnerBinaryName      = "mac-runner-aarch64"
	RunnerBinaryAvailable = false
)
