// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"encoding/json"
	"io"

	"github.com/exasol/exasol-personal/internal/config"
)

func DumpDeploymentInfo(
	ctx context.Context,
	deployment config.DeploymentDir,
	writer io.Writer,
) error {
	return withDeploymentSharedLock(ctx, deployment, func(deployment config.DeploymentDir) error {
		return dumpDeploymentInfoUnsafe(deployment, writer)
	})
}

func dumpDeploymentInfoUnsafe(deployment config.DeploymentDir, writer io.Writer) error {
	details, err := config.ReadNodeDetails(deployment)
	if err != nil {
		return err
	}

	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")

	return encoder.Encode(details)
}
