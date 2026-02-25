// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"encoding/json"
	"io"

	"github.com/exasol/exasol-personal/internal/config"
)

func DumpDeploymentInfo(ctx context.Context, deploymentDir string, writer io.Writer) error {
	return withDeploymentSharedLock(ctx, deploymentDir, func(dir string) error {
		return dumpDeploymentInfoUnsafe(dir, writer)
	})
}

func dumpDeploymentInfoUnsafe(deploymentDir string, writer io.Writer) error {
	details, err := config.ReadNodeDetails(deploymentDir)
	if err != nil {
		return err
	}

	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")

	return encoder.Encode(details)
}
