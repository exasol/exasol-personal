// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

//go:build linux && amd64

package tofubin

import _ "embed"

//go:embed generated/linux/amd64/tofu
var TofuBinary []byte

const TofuBinaryName = "tofu"

//go:embed generated/amd64/alpine-amd64.qcow2
var AlpineImage []byte

const AlpineImageName = "alpine-amd64.qcow2"

//go:embed generated/cloud-init.iso
var CloudInitImage []byte

const CloudInitImageName = "cloud-init.iso"

//go:embed generated/vm-key
var VmSshKey []byte

const VmSshKeyName = "vm-key"
