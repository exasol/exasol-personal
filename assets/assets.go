// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package assets

import (
	"embed"
)

//go:embed all:infrastructure/**
var InfrastructureAssets embed.FS

//go:embed all:installation/**
var InstallationAssets embed.FS

//go:embed all:shared/**
var SharedAssets embed.FS

const (
	InfrastructureAssetDir = "infrastructure"
	InstallationAssetDir   = "installation"
	SharedAssetDir         = "shared"
)
