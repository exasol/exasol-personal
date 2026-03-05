// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"crypto/rand"
	"encoding/hex"
	"path/filepath"
	"strings"
)

const (
	// DeploymentIdPrefix matches the historical prefix used by infrastructure presets.
	clusterIdentityPrefix = "exasol-personal"
)

// GenerateDeploymentId creates a short, stable identifier for a deployment.
//
// Design goal: keep it readable and compatible with existing preset conventions.
// The generated ID has the form "exasol-" + 8 lowercase hex characters.
func GenerateDeploymentId() (string, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}

	return hex.EncodeToString(b[:]), nil
}

func presetIdentityToken(p PresetRef) string {
	var tok string
	if p.IsPath() {
		tok = filepath.Base(filepath.Clean(strings.TrimSpace(p.Path)))
	} else {
		tok = strings.TrimSpace(p.Name)
	}
	if tok == "" {
		return "unknown"
	}
	// Prevent delimiter injection into the semicolon-separated identity.
	tok = strings.ReplaceAll(tok, ";", "_")
	// Keep tokens compact and shell/URL friendly.
	tok = strings.ReplaceAll(tok, " ", "_")

	return tok
}

// ComputeClusterIdentity returns the launcher-governed cluster identity string.
//
// The string is treated as opaque by presets and scripts.
func ComputeClusterIdentity(
	deploymentId string,
	infraPreset PresetRef,
	installPreset PresetRef,
) string {
	deploymentId = strings.TrimSpace(deploymentId)
	if deploymentId == "" {
		deploymentId = "unknown"
	}

	return strings.Join([]string{
		clusterIdentityPrefix,
		deploymentId,
		presetIdentityToken(infraPreset),
		presetIdentityToken(installPreset),
	}, ";")
}
