// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"

	"github.com/exasol/exasol-personal/internal/config"
)

func GetDiagnosticDeploymentInfo(
	ctx context.Context,
	deployment config.DeploymentDir,
) (*config.DeploymentInfo, error) {
	var details *config.DeploymentInfo
	err := withDeploymentSharedLock(ctx, deployment, func(deployment config.DeploymentDir) error {
		var readErr error
		details, readErr = config.ReadDeploymentInfo(deployment)

		return readErr
	})

	return details, err
}
