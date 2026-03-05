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
	"github.com/exasol/exasol-personal/internal/presets"
	"github.com/exasol/exasol-personal/internal/remote"
	"github.com/exasol/exasol-personal/internal/task_runner"
	"github.com/exasol/exasol-personal/internal/tofu"
	"github.com/exasol/exasol-personal/internal/util"
	"github.com/gobwas/glob"
)

// TofuLockfileMode controls whether OpenTofu is allowed to update .terraform.lock.hcl during init.
//
// This is an alias to the enum in the tofu package to avoid exposing internal/tofu details
// to the CLI layer, while still using a strongly typed enum throughout the deploy pipeline.
type TofuLockfileMode = tofu.LockfileMode

const (
	TofuLockfileReadonly = tofu.LockfileReadonly
	TofuLockfileUpdate   = tofu.LockfileUpdate
)

const (
	deployFailureResourceHint = "Deployment may have created cloud resources " +
		"that can incur costs. " +
		"Fix the problem and run `deploy` again, or run `destroy` to clean up."
)

func appendDeployFailureResourceHint(err error) error {
	if err == nil {
		return nil
	}

	return fmt.Errorf("%w\n\n%s", err, deployFailureResourceHint)
}

//nolint:revive
func Deploy(
	ctx context.Context,
	deploymentDir string,
	verbose bool,
	tofuLockfileMode TofuLockfileMode,
) error {
	slog.Debug("Running deploy")

	// Execute according to infrastructure/installation manifests instead of exasolConfig.yaml
	return deployFromManifests(ctx, deploymentDir, verbose, tofuLockfileMode)
}

func WorkflowStatePermitsDeploy(
	exasolState *config.ExasolPersonalState,
	deploymentDir string,
) error {
	// Check we are in state init, inprogress or deploymentInterrupted.
	// - We are allowed to retry deployment when in the interrupted state, because
	//   tofu apply is idempotent
	workflowState, err := exasolState.GetWorkflowState()
	if err != nil {
		slog.Error("failed to read workflow state")
		return err
	}

	switch state := workflowState.(type) {
	case *config.WorkflowStateInitialized,
		*config.WorkflowStateDeploymentFailed,
		*config.WorkflowStateRunning:
		return nil

	case *config.WorkflowStateOperationInProgress:
		if state.Operation == config.DeployOperation {
			return nil
		}

		return ErrUnspportedOperation

	case *config.WorkflowStateInterrupted:
		if state.InterruptedDuringOperation == config.DeployOperation {
			slog.Debug("deploying in workflow state `deploymentInterrupted`")
			return nil
		}
	}

	LogDeploymentStatus(deploymentDir)

	return ErrUnexpectedDeploymentStatus
}

//
//nolint:revive
func deployFromManifests(
	ctx context.Context,
	deploymentDir string,
	verbose bool,
	tofuLockfileMode TofuLockfileMode,
) error {
	return withDeploymentExclusiveLock(ctx, deploymentDir,
		func(deploymentDir string) error {
			exasolState, err := config.ReadExasolPersonalState(deploymentDir)
			if err != nil {
				slog.Error("failed to read exasol personal state")
				return err
			}

			if err := WorkflowStatePermitsDeploy(exasolState, deploymentDir); err != nil {
				return util.LoggedError(err, "run `status` for more information")
			}

			// Set the workflowstate to deployment in-progress
			err = exasolState.SetWorkflowStateAndWrite(&config.WorkflowStateOperationInProgress{
				Operation: config.DeployOperation,
			}, deploymentDir)
			if err != nil {
				slog.Error("failed to set workflow state to in-progress", "error", err.Error())
			}

			// Register signal handler for catching interruptions and set state
			// in case of interruption
			unregister, _ := util.RegisterOnceSignalHandler(func() {
				slog.Warn("Deployment interrupted")
				err = exasolState.SetWorkflowStateAndWrite(&config.WorkflowStateInterrupted{
					Error:                      "Deployment interrupted via signal",
					InterruptedDuringOperation: config.DeployOperation,
				}, deploymentDir)
				if err != nil {
					slog.Error("failed to set workflow state to in-progress", "error", err.Error())
				}
			})

			// Fallback cleanup
			defer unregister()

			// Manifest-driven execution
			infrastructureManifest, err := config.ReadInfrastructureManifest(deploymentDir)
			if err != nil {
				return err
			}

			installManifest, err := config.ReadInstallManifest(deploymentDir)
			if err != nil {
				return err
			}

			var externalCommandOutput io.Writer
			if verbose {
				externalCommandOutput = os.Stderr
			}

			// Provision infrastructure via tofu when declared in infrastructure manifest
			if err := runInfrastructureProvision(
				ctx,
				deploymentDir,
				infrastructureManifest,
				externalCommandOutput,
				externalCommandOutput,
				tofuLockfileMode,
			); err != nil {
				unregister()

				deployErr := appendDeployFailureResourceHint(err)
				if stateErr := exasolState.SetWorkflowStateAndWrite(
					&config.WorkflowStateDeploymentFailed{
						Error: deployErr.Error(),
					},
					deploymentDir,
				); stateErr != nil {
					slog.Warn("failed to persist deployment failure state", "error", stateErr)
				}

				return deployErr
			}

			// Installation phase (remoteExec tasks)
			if err := runInstallSteps(ctx, deploymentDir, installManifest,
				externalCommandOutput, externalCommandOutput); err != nil {
				unregister()
				deployErr := appendDeployFailureResourceHint(err)
				if stateErr := exasolState.SetWorkflowStateAndWrite(
					&config.WorkflowStateDeploymentFailed{
						Error: deployErr.Error(),
					},
					deploymentDir,
				); stateErr != nil {
					slog.Warn("failed to persist deployment failure state", "error", stateErr)
				}

				return deployErr
			}

			// Stop handling interrupts before committing success state
			unregister()

			err = exasolState.SetWorkflowStateAndWrite(
				&config.WorkflowStateRunning{},
				deploymentDir,
			)
			if err != nil {
				slog.Error("failed to write workflow state")
				return err
			}

			connectionInstructions, err := getConnectionInstructionsTextUnsafe(ctx, deploymentDir)
			if err != nil {
				slog.Error("failed to collect connection instructions")
				return err
			}
			if err := writeConnectionInstructionsFile(deploymentDir, connectionInstructions); err != nil {
				slog.Error("failed to write connection instructions")
				return err
			}

			slog.Info("Completed deploying")

			return nil
		})
}

type nodeLookupImpl struct {
	deploymentDir string
}

var _ task_runner.NodeLookup = &nodeLookupImpl{}

func NewNodeLookup(deploymentDir string) task_runner.NodeLookup {
	return &nodeLookupImpl{
		deploymentDir: deploymentDir,
	}
}

func (s *nodeLookupImpl) Find(
	nodeNameGlob string,
) ([]task_runner.RunScriptNode, error) {
	nodeListBuilder, err := newNodeListBuilder(s.deploymentDir)
	if err != nil {
		return nil, util.LoggedError(err, "failed to create node list builder")
	}

	nodes, err := nodeListBuilder.BuildForNodeGlob(nodeNameGlob)
	if err != nil {
		return nil, util.LoggedError(err, "failed to build node list", "node", nodeNameGlob)
	}

	return nodes, nil
}

type nodeListBuilder struct {
	deploymentDir string
	nodeDetails   *config.NodeDetails
}

func newNodeListBuilder(deploymentDir string) (*nodeListBuilder, error) {
	absDeploymentDir, err := filepath.Abs(deploymentDir)
	if err != nil {
		return nil, err
	}
	nodeDetails, err := config.ReadNodeDetails(absDeploymentDir)
	if err != nil {
		return nil, err
	}

	return &nodeListBuilder{
		deploymentDir: absDeploymentDir,
		nodeDetails:   nodeDetails,
	}, nil
}

func (builder *nodeListBuilder) BuildForNodeGlob(
	nodeGlob string,
) ([]task_runner.RunScriptNode, error) {
	search, err := glob.Compile(nodeGlob)
	if err != nil {
		return nil, err
	}

	result := []task_runner.RunScriptNode{}

	for _, nodeName := range builder.nodeDetails.ListNodes() {
		if search.Match(nodeName) {
			remoteConn, err := builder.getRemote(nodeName)
			if err != nil {
				return nil, err
			}

			result = append(result, task_runner.RunScriptNode{
				Name:              nodeName,
				ConnectionOptions: remoteConn,
			})
		}
	}

	return result, nil
}

func (builder *nodeListBuilder) getRemote(node string) (*remote.SSHConnectionOptions, error) {
	sshDetails, err := builder.nodeDetails.GetSSHDetails(node)
	if err != nil {
		return nil, err
	}

	keyFilePath := sshDetails.KeyFile
	if !filepath.IsAbs(keyFilePath) {
		keyFilePath = filepath.Join(builder.deploymentDir, keyFilePath)
	}

	keyData, err := os.ReadFile(keyFilePath)
	if err != nil {
		return nil, fmt.Errorf("%w: could not read SSH key file %s", err, keyFilePath)
	}

	return &remote.SSHConnectionOptions{
		Host: sshDetails.Host,
		User: sshDetails.User,
		Port: sshDetails.Port,
		Key:  keyData,
	}, nil
}

// Helpers to execute manifests

func runInfrastructureProvision(
	ctx context.Context,
	deploymentDir string,
	manifest *presets.InfrastructureManifest,
	out, outErr io.Writer,
	tofuLockfileMode TofuLockfileMode,
) error {
	if manifest.Tofu == nil {
		slog.Info("tofu: no configuration defined; skipping")
		return nil
	}

	slog.Info("beginning deployment with tofu")

	tofuCfg := tofu.NewTofuConfigFromDeployment(deploymentDir, *manifest.Tofu)
	logBuffer := task_runner.NewLogBuffer()
	stdOutWriter := util.CombineWriters(logBuffer, out)
	stdErrWriter := util.CombineWriters(logBuffer, outErr)

	if err := tofu.Initialize(ctx, *tofuCfg,
		stdOutWriter, stdErrWriter, tofuLockfileMode); err != nil {
		logBuffer.ReplayLogMessages(ctx)
		slog.Error("tofu: failed to init", "error", err)

		return err
	}

	if err := tofu.Plan(ctx, *tofuCfg, stdOutWriter, stdErrWriter); err != nil {
		logBuffer.ReplayLogMessages(ctx)
		slog.Error("tofu: failed to plan")

		return err
	}

	if err := tofu.ApplyPlan(ctx, *tofuCfg, stdOutWriter, stdErrWriter); err != nil {
		logBuffer.ReplayLogMessages(ctx)
		slog.Error("tofu: failed to apply")

		return err
	}

	return nil
}

// Helpers to execute manifests

func runInstallSteps(
	ctx context.Context,
	deploymentDir string,
	im *presets.InstallManifest,
	out, outErr io.Writer,
) error {
	// Translate remoteExec steps to tasks
	tasks := []config.Task{}
	for _, step := range im.Install {
		if step.RemoteExec != nil {
			// The filenames in the installation manifest are relative
			// to the installation directory.
			// Make them relative to the deployment directory here.
			if step.RemoteExec.Filename != "" {
				step.RemoteExec.Filename = filepath.Join("installation", step.RemoteExec.Filename)
			}
			tasks = append(tasks, config.Task{RemoteExec: step.RemoteExec})
		}
	}
	if len(tasks) == 0 {
		slog.Info("no installation steps defined; skipping")
		return nil
	}

	return task_runner.NewTaskRunner(
		&task_runner.LocalCommandRunnerImpl{},
		&task_runner.RemoteScriptRunnerImpl{},
		NewNodeLookup(deploymentDir),
	).RunTasks(ctx, tasks, deploymentDir, out, outErr)
}
