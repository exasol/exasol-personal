// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

//go:build darwin && arm64

package tofubin

import _ "embed"

//go:embed generated/darwin/arm64/tofu
var TofuBinary []byte

const TofuBinaryName = "tofu"

//go:embed generated/arm64/alpine-arm64.qcow2
var AlpineImage []byte

const AlpineImageName = "alpine-arm64.qcow2"

//go:embed generated/cloud-init.iso
var CloudInitImage []byte

const CloudInitImageName = "cloud-init.iso"

//go:embed generated/vm-key
var VmSshKey []byte

const VmSshKeyName = "vm-key"
