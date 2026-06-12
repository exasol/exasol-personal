// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"fmt"

	"github.com/exasol/exasol-personal/assets"
)

const localContainerShellScriptAssetPath = "scripts/container_shell.sh"

func readLocalAsset(assetPath string) (string, error) {
	content, err := assets.LocalAssets.ReadFile(assets.LocalAssetDir + "/" + assetPath)
	if err != nil {
		return "", fmt.Errorf("failed to read local asset %q: %w", assetPath, err)
	}

	return string(content), nil
}
