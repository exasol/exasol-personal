// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/exasol/exasol-personal/assets"
)

const (
	localContainerShellScriptAssetPath      = "scripts/container_shell.sh"
	localShellInstructionsTemplateAssetPath = "templates/local_shell_instructions.txt"
)

func readLocalAsset(assetPath string) (string, error) {
	content, err := assets.LocalAssets.ReadFile(assets.LocalAssetDir + "/" + assetPath)
	if err != nil {
		return "", fmt.Errorf("failed to read local asset %q: %w", assetPath, err)
	}

	return string(content), nil
}

func renderLocalAssetTemplate(assetPath string, data any) (string, error) {
	content, err := readLocalAsset(assetPath)
	if err != nil {
		return "", err
	}

	tmpl, err := template.New(assetPath).Parse(content)
	if err != nil {
		return "", fmt.Errorf("failed to parse local asset %q: %w", assetPath, err)
	}

	var output bytes.Buffer
	if err := tmpl.Execute(&output, data); err != nil {
		return "", fmt.Errorf("failed to render local asset %q: %w", assetPath, err)
	}

	return output.String(), nil
}
