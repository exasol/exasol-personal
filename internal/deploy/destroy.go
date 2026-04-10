// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localruntime"
	"github.com/exasol/exasol-personal/internal/task_runner"
	"github.com/exasol/exasol-personal/internal/tofu"
	"github.com/exasol/exasol-personal/internal/util"
)

//nolint:revive
func Destroy(ctx context.Context, deploymentDir string, verbose bool) error {
	return withDeploymentExclusiveLock(ctx, deploymentDir,
		func(deploymentDir string) error {
			slog.Info("Destroying deployment and releasing all resources")

			exasolState, err := config.ReadExasolPersonalState(deploymentDir)
			if err != nil {
				return err
			}

			// Set the workflowstate to destroy in-progress
			err = exasolState.SetWorkflowStateAndWrite(&config.WorkflowStateOperationInProgress{
				Operation: config.DestroyOperation,
			}, deploymentDir)
			if err != nil {
				slog.Error("failed to set workflow state to in-progress", "error", err.Error())
			}

			// Register signal handler for catching interruptions and set state
			// in case of interruption
			unregister, _ := util.RegisterOnceSignalHandler(func() {
				slog.Warn("Destroy interrupted")
				_ = exasolState.SetWorkflowStateAndWrite(&config.WorkflowStateInterrupted{
					Error:                      "Destroy interrupted via signal",
					InterruptedDuringOperation: config.DestroyOperation,
				}, deploymentDir)
			})

			defer unregister()

			manifest, err := config.ReadInfrastructureManifest(deploymentDir)
			if err != nil {
				return err
			}

			var externalCommandStandardOut io.Writer
			if verbose {
				externalCommandStandardOut = os.Stderr
			}

			if manifest.LocalRuntime != nil {
				slog.Info("destroying local runtime resources")

				backend, err := localruntime.NewBackend(localruntime.Config{
					Kind: manifest.LocalRuntime.Kind,
				}, &localruntime.LocalExecutor{})
				if err != nil {
					return err
				}

				if err := backend.Destroy(ctx, localruntime.DestroyOptions{
					DeploymentDir: deploymentDir,
				}); err != nil {
					return err
				}
			} else if manifest.Tofu != nil {
				tofuCfg := tofu.NewTofuConfigFromDeployment(deploymentDir, *manifest.Tofu)
				logBuffer := task_runner.NewLogBuffer()
				err = tofu.Destroy(
					ctx,
					*tofuCfg,
					util.CombineWriters(logBuffer, externalCommandStandardOut),
					util.CombineWriters(logBuffer, externalCommandStandardOut),
				)
				if err != nil {
					logBuffer.ReplayLogMessages(ctx)
					slog.Error("failed to destroy cloud resources")

					return err
				}
			} else {
				slog.Info("no infrastructure provisioning configuration defined; skipping destroy")
			}

			// Stop handling interrupts before committing final initialized state
			unregister()

			// Returning to the initialized state is required so that `deploy` can be run again.
			err = exasolState.SetWorkflowStateAndWrite(
				&config.WorkflowStateInitialized{},
				deploymentDir,
			)
			if err != nil {
				return err
			}

			err = os.Remove(filepath.Join(deploymentDir, config.ConnectionInstruction))
			if err != nil {
				slog.Debug(fmt.Sprintf("failed to remove connection instructions file: %v", err))
			}
			if err := os.Remove(filepath.Join(deploymentDir, localruntime.MetadataFileName)); err != nil &&
				!os.IsNotExist(err) {
				slog.Debug(fmt.Sprintf("failed to remove local runtime metadata file: %v", err))
			}

			slog.Info("Successfully destroyed deployment and released all resources")

			return nil
		})
}
