// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/exasol/exasol-personal/internal/presets"
	"github.com/exasol/exasol-personal/internal/runtimeartifacts"
)

// IsExternalPresetURI reports whether arg looks like an external preset URI
// that should be resolved via ResolvePreset rather than treated as an embedded
// preset name or local filesystem path.
func IsExternalPresetURI(arg string) bool {
	return strings.HasPrefix(arg, "file://") ||
		strings.HasPrefix(arg, "http://") ||
		strings.HasPrefix(arg, "https://") ||
		strings.HasPrefix(arg, "git://") ||
		strings.HasPrefix(arg, "git@")
}

// ResolvePreset resolves an external preset URI to a local directory path and
// verifies the expected manifest file is present for the given preset type.
func ResolvePreset(
	ctx context.Context,
	manager *runtimeartifacts.Manager,
	uri string,
	presetType string,
) (string, error) {
	repoURL, ref := runtimeartifacts.ParseGitURL(uri)
	if ref != "" && !runtimeartifacts.IsGitSourceURL(repoURL) {
		return "", fmt.Errorf(
			"@ref syntax (%q) is only valid on git source URLs;"+
				" %q does not appear to be a git repository",
			ref,
			repoURL,
		)
	}

	def := runtimeartifacts.ResourceDefinition{
		Extract: needsExtraction(uri),
		Artifact: map[string]runtimeartifacts.ArtifactSpec{
			"any": {URL: uri},
		},
	}

	resolvedPath, err := manager.Get(ctx, def, presetType)
	if err != nil {
		return "", fmt.Errorf("resolving preset %q: %w", uri, err)
	}

	return resolvedPath, verifyPresetManifest(resolvedPath, presetType, uri)
}

func needsExtraction(url string) bool {
	return strings.HasSuffix(url, ".tar.gz") ||
		strings.HasSuffix(url, ".tgz") ||
		strings.HasSuffix(url, ".zip")
}

func verifyPresetManifest(resolvedPath, presetType, uri string) error {
	manifestFile := manifestFilenameFor(presetType)
	if manifestFile == "" {
		return nil
	}
	manifestPath := filepath.Join(resolvedPath, manifestFile)
	if _, err := os.Stat(manifestPath); err != nil {
		return fmt.Errorf(
			"preset %q resolved to %q but does not contain the expected %s manifest (%s)",
			uri, resolvedPath, presetType, manifestFile,
		)
	}

	return nil
}

func manifestFilenameFor(presetType string) string {
	switch presetType {
	case presets.PresetTypeInfrastructure:
		return presets.InfrastructureManifestFilename
	case presets.PresetTypeInstallation:
		return presets.InstallationManifestFilename
	default:
		return ""
	}
}
