// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"os"
	"text/template"

	"github.com/exasol/exasol-personal/internal/config"
)

const connectionInstructionsFileMode = 0o600

//go:embed connection_instructions.tmpl
var connectionInstructionsTemplateText string

var connectionInstructionsTemplate = template.Must(
	template.New("connection-instructions").Parse(connectionInstructionsTemplateText),
)

func GetConnectionInstructionsText(
	ctx context.Context,
	deployment config.DeploymentDir,
) (string, error) {
	report, err := GetDeploymentInfoReport(ctx, deployment)
	if err != nil {
		return "", err
	}

	return RenderConnectionInstructionsText(report)
}

func getConnectionInstructionsTextUnsafe(
	ctx context.Context,
	deployment config.DeploymentDir,
) (string, error) {
	report, err := getDeploymentInfoReportWithExistingLock(ctx, deployment)
	if err != nil {
		return "", err
	}

	return RenderConnectionInstructionsText(report)
}

func RenderConnectionInstructionsText(report *DeploymentInfoReport) (string, error) {
	var output bytes.Buffer
	if err := connectionInstructionsTemplate.Execute(&output, report); err != nil {
		return "", err
	}

	return output.String(), nil
}

func writeConnectionInstructionsFile(deployment config.DeploymentDir, content string) error {
	err := os.WriteFile(
		deployment.ConnectionInstructionsPath(),
		[]byte(content),
		connectionInstructionsFileMode,
	)
	if err != nil {
		return fmt.Errorf("failed to write instructions to file: %w", err)
	}

	return nil
}
