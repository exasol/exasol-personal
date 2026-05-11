// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import "github.com/exasol/exasol-personal/internal/tofu"

// TofuLockfileMode controls whether OpenTofu is allowed to update .terraform.lock.hcl during init.
//
// This is an alias to the enum in the tofu package to avoid exposing internal/tofu details
// to the CLI layer, while still using a strongly typed enum throughout the deploy pipeline.
type TofuLockfileMode = tofu.LockfileMode

const (
	TofuLockfileReadonly = tofu.LockfileReadonly
	TofuLockfileUpdate   = tofu.LockfileUpdate
)
