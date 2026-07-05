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
	"github.com/exasol/exasol-personal/internal/util"
	"github.com/gobwas/glob"
)

const (
	cloudDeployFailureResourceHint = "Deployment may have created cloud resources " +
		"that can incur costs. " +
		"Fix the problem and run `deploy` again, or run `destroy` to clean up."
	localDeployFailureResourceHint = "Deployment may have created local VM runtime files. " +
		"Fix the problem and run `deploy` again, or run `destroy` to clean up."
)

func appendDeployFailureHint(
	deployment config.DeploymentDir,
	err error,
) error {
	if err == nil {
		return nil
	}

	return fmt.Errorf(
		"%w\n\nInspect launcher logs at %s for details. %s%s",
		err,
		deployment.Resolve("deployment.log"),
		deployFailureResourceHint(deployment),
		deployFailureResourceHintSuffix(deployment),
	)
}

func deployFailureResourceHint(deployment config.DeploymentDir) string {
	manifest, err := config.ReadInfrastructureManifest(deployment)
	if err != nil {
		return cloudDeployFailureResourceHint
	}
	backend, err := resolveBackendKind(manifest)
	if err != nil {
		return cloudDeployFailureResourceHint
	}
	if backend == backendTypeLocal {
		return localDeployFailureResourceHint
	}

	return cloudDeployFailureResourceHint
}

func deployFailureResourceHintSuffix(deployment config.DeploymentDir) string {
	if _, statErr := os.Stat(deployment.NodeDetailsPath()); statErr == nil {
		return fmt.Sprintf(
			" Additional deployment diagnostics are stored in %s.",
			deployment.NodeDetailsPath(),
		)
	}

	return ""
}

//nolint:revive
func Deploy(
	ctx context.Context,
	deployment config.DeploymentDir,
	verbose bool,
	options DeployOptions,
) error {
	slog.Debug("Running deploy")

	// Execute according to infrastructure/installation manifests instead of exasolConfig.yaml
	return deployFromManifests(ctx, deployment, verbose, options)
}

func WorkflowStatePermitsDeploy(
	exasolState *config.ExasolPersonalState,
	deployment config.DeploymentDir,
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

		return newBlockedDeployError(deployment, ErrUnspportedOperation)

	case *config.WorkflowStateInterrupted:
		if state.InterruptedDuringOperation == config.DeployOperation {
			slog.Debug("deploying in workflow state `deploymentInterrupted`")
			return nil
		}
	}

	return newBlockedDeployError(deployment, ErrUnexpectedDeploymentStatus)
}

// blockedDeployError carries an actionable, user-facing message for a deployment state that
// does not permit deploy, while still unwrapping to the underlying sentinel so errors.Is
// checks keep working. Its message deliberately does not surface the sentinel's terse text,
// so a known, recoverable state (e.g. interrupted, stopped) is not labelled "unexpected".
type blockedDeployError struct {
	sentinel error
	message  string
}

func (e *blockedDeployError) Error() string { return e.message }
func (e *blockedDeployError) Unwrap() error { return e.sentinel }

// newBlockedDeployError builds an actionable error for a deployment state that does not
// permit deploy. It reuses the same recovery guidance `exasol status` reports (see
// GetStatus), so the user is told the current state and how to recover without having to
// run a second command.
//
//nolint:contextcheck // guard has no context; a background read matches LogDeploymentStatus
func newBlockedDeployError(deployment config.DeploymentDir, sentinel error) error {
	status, err := GetStatus(context.Background(), deployment, false)
	if err != nil || status == nil {
		slog.Error("failed to get deployment status for recovery guidance", "error", err)

		return sentinel
	}

	guidance := status.Message
	if guidance == "" {
		guidance = "Run `exasol status` for details."
	}

	slog.Warn("deployment cannot be deployed in its current state",
		"status", status.Status, "deployment_dir", deployment.Root())

	return &blockedDeployError{
		sentinel: sentinel,
		message: fmt.Sprintf(
			"deployment in %s is in state %q and cannot be deployed right now. %s",
			deployment.Root(), status.Status, guidance,
		),
	}
}

//
//nolint:revive
func deployFromManifests(
	ctx context.Context,
	deployment config.DeploymentDir,
	verbose bool,
	options DeployOptions,
) error {
	return withDeploymentExclusiveLock(ctx, deployment,
		func(deployment config.DeploymentDir) error {
			exasolState, err := config.ReadExasolPersonalState(deployment)
			if err != nil {
				slog.Error("failed to read exasol personal state")
				return err
			}

			if err := WorkflowStatePermitsDeploy(exasolState, deployment); err != nil {
				return err
			}

			// Set the workflowstate to deployment in-progress
			err = exasolState.SetWorkflowStateAndWrite(&config.WorkflowStateOperationInProgress{
				Operation: config.DeployOperation,
			}, deployment)
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
				}, deployment)
				if err != nil {
					slog.Error("failed to set workflow state to in-progress", "error", err.Error())
				}
			})

			// Fallback cleanup
			defer unregister()

			// Manifest-driven execution
			infrastructureManifest, err := config.ReadInfrastructureManifest(deployment)
			if err != nil {
				return err
			}
			backend, err := newDeploymentBackend(deployment, infrastructureManifest)
			if err != nil {
				return err
			}

			installManifest, err := config.ReadInstallManifest(deployment)
			if err != nil {
				return err
			}

			var externalCommandOutput io.Writer
			if verbose {
				externalCommandOutput = os.Stderr
			}

			if err := backend.Deploy(
				ctx,
				externalCommandOutput,
				externalCommandOutput,
				options,
			); err != nil {
				unregister()

				deployErr := appendDeployFailureHint(deployment, err)
				if stateErr := exasolState.SetWorkflowStateAndWrite(
					&config.WorkflowStateDeploymentFailed{
						Error: deployErr.Error(),
					},
					deployment,
				); stateErr != nil {
					slog.Warn("failed to persist deployment failure state", "error", stateErr)
				}

				return deployErr
			}

			// Installation phase (remoteExec tasks)
			if err := runInstallSteps(ctx, deployment, installManifest,
				externalCommandOutput, externalCommandOutput); err != nil {
				unregister()
				deployErr := appendDeployFailureHint(deployment, err)
				if stateErr := exasolState.SetWorkflowStateAndWrite(
					&config.WorkflowStateDeploymentFailed{
						Error: deployErr.Error(),
					},
					deployment,
				); stateErr != nil {
					slog.Warn("failed to persist deployment failure state", "error", stateErr)
				}

				return deployErr
			}

			// Stop handling interrupts before committing success state
			unregister()

			err = exasolState.SetWorkflowStateAndWrite(
				&config.WorkflowStateRunning{},
				deployment,
			)
			if err != nil {
				slog.Error("failed to write workflow state")
				return err
			}

			connectionInstructions, err := getConnectionInstructionsTextUnsafe(ctx, deployment)
			if err != nil {
				slog.Error("failed to collect connection instructions")
				return err
			}
			if err := writeConnectionInstructionsFile(deployment, connectionInstructions); err != nil {
				slog.Error("failed to write connection instructions")
				return err
			}

			slog.Info("Completed deploying")

			return nil
		})
}

type nodeLookupImpl struct {
	deployment config.DeploymentDir
}

var _ task_runner.NodeLookup = &nodeLookupImpl{}

func NewNodeLookup(deployment config.DeploymentDir) task_runner.NodeLookup {
	return &nodeLookupImpl{
		deployment: deployment,
	}
}

func (s *nodeLookupImpl) Find(
	nodeNameGlob string,
) ([]task_runner.RunScriptNode, error) {
	nodeListBuilder, err := newNodeListBuilder(s.deployment)
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
	deployment  config.DeploymentDir
	nodeDetails *config.NodeDetails
}

func newNodeListBuilder(deployment config.DeploymentDir) (*nodeListBuilder, error) {
	nodeDetails, err := config.ReadNodeDetails(deployment)
	if err != nil {
		return nil, err
	}

	return &nodeListBuilder{
		deployment:  deployment,
		nodeDetails: nodeDetails,
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
	sshDetails, err := builder.nodeDetails.GetSSHDetails(node, builder.deployment)
	if err != nil {
		return nil, err
	}

	keyFilePath := sshDetails.KeyFile
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

func runInstallSteps(
	ctx context.Context,
	deployment config.DeploymentDir,
	im *presets.InstallManifest,
	out, outErr io.Writer,
) error {
	tasks := buildInstallTasks(im)
	if len(tasks) == 0 {
		slog.Info("no installation steps defined; skipping")
		return nil
	}

	return task_runner.NewTaskRunner(
		&task_runner.LocalCommandRunnerImpl{},
		&task_runner.RemoteScriptRunnerImpl{},
		NewNodeLookup(deployment),
	).RunTasks(ctx, tasks, deployment, out, outErr)
}

func buildInstallTasks(installManifest *presets.InstallManifest) []config.Task {
	if installManifest == nil {
		return nil
	}

	tasks := []config.Task{}
	for _, step := range installManifest.Install {
		if step.RemoteExec != nil {
			remoteExec := *step.RemoteExec
			if remoteExec.Filename != "" {
				remoteExec.Filename = filepath.Join("installation", remoteExec.Filename)
			}
			tasks = append(tasks, config.Task{RemoteExec: &remoteExec})
		}
		if step.LocalCommand != nil {
			localCommand := *step.LocalCommand
			tasks = append(tasks, config.Task{LocalCommand: &localCommand})
		}
	}

	return tasks
}
