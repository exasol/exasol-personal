// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"fmt"

	"github.com/exasol/exasol-personal/assets"
)

const localContainerShellScriptAssetPath = "local/files/container_shell.sh"

func readLocalInfrastructureAsset(assetPath string) (string, error) {
	content, err := assets.InfrastructureAssets.ReadFile(
		assets.InfrastructureAssetDir + "/" + assetPath,
	)
	if err != nil {
		return "", fmt.Errorf("failed to read local infrastructure asset %q: %w", assetPath, err)
	}

	return string(content), nil
}
