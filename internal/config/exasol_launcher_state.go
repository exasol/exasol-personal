// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT
package config

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// ExasolPersonalStateFileName is the persistent workflow/metadata state file stored
// in a deployment directory.
const ExasolPersonalStateFileName = ".exasolLauncherState.json"

type (
	WorkflowStateInitialized struct{}
	WorkflowStateRunning     struct{}
	WorkflowStateStopped     struct{}
)

type WorkflowStateOperationInProgress struct {
	Operation string `json:"operation"`
}

const (
	DeployOperation  = "deploy"
	DestroyOperation = "destroy"
	StartOperation   = "start"
	StopOperation    = "stop"
)

type WorkflowStateInterrupted struct {
	Error                      string `json:"error"`
	InterruptedDuringOperation string `json:"interruptedDuringOperation"`
}

type WorkflowStateDeploymentFailed struct {
	Error string `json:"error"`
}

type WorkflowState struct {
	// Union. Exactly one field should be set.
	Initialized         *WorkflowStateInitialized         `json:"initialized,omitempty"`
	OperationInProgress *WorkflowStateOperationInProgress `json:"operationInProgress,omitempty"`
	Interrupted         *WorkflowStateInterrupted         `json:"interrupted,omitempty"`
	DeploymentFailed    *WorkflowStateDeploymentFailed    `json:"deploymentFailed,omitempty"`
	Running             *WorkflowStateRunning             `json:"running,omitempty"`
	Stopped             *WorkflowStateStopped             `json:"stopped,omitempty"`
}

type ExasolPersonalState struct {
	CurrentWorkflowState WorkflowState `json:"currentWorkflowState"`
	// DeploymentId is a launcher-governed stable identifier for this deployment.
	//
	// It can be injected into infrastructure presets (e.g. for tagging) and is also
	// used as a component of cluster identity for version checking.
	DeploymentId string `json:"deploymentId,omitempty"`
	// ClusterIdentity is a launcher-governed stable identity string for this deployment.
	// Presets and scripts should treat it as opaque.
	ClusterIdentity string `json:"clusterIdentity,omitempty"`
	// DeploymentVersion is the launcher version that created this deployment directory.
	//
	// Note: the long-term, stable deployment-version marker used for compatibility
	// checks is the plain-text file ".exasolLauncher.version".
	// This JSON field mirrors that marker for convenience, but compatibility checks
	// should not depend on it.
	DeploymentVersion   string    `json:"deploymentVersion"`
	LastVersionCheck    time.Time `json:"lastVersionCheck"`
	VersionCheckEnabled bool      `json:"versionCheckEnabled"`
}

// DirectoryExasolPersonalStatefile.
func IsDirectoryContainingStateFile(directory string) (bool, error) {
	path := filepath.Join(directory, ExasolPersonalStateFileName)

	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}

		return false, err
	}

	if info.IsDir() {
		// The path exists but is a directory, not a file.
		return false, nil
	}

	return true, nil
}

// SetExasolPersonalState writes a new exasol personal state to permanent storage.
func WriteExasolPersonalState(state *ExasolPersonalState, deploymentDir string) error {
	return writeConfig(state,
		filepath.Join(deploymentDir, ExasolPersonalStateFileName),
		"exasol personal state")
}

var ErrNoExasolPersonalStateSet = errors.New("no exasol-personal state is set")

// GetExasolPersonalState returns the deployment state of the deployment directory.
func ReadExasolPersonalState(deploymentDir string) (*ExasolPersonalState, error) {
	slog.Debug("reading exasol personal state")
	state, err := readConfig[ExasolPersonalState](
		filepath.Join(deploymentDir, ExasolPersonalStateFileName),
		"exasol personal state")
	if err != nil {
		return nil, err
	}

	return state, nil
}

// SetWorkflowState writes a new workflow state to permanent storage. The passed struct must be one
// of the WorkflowState* structs.
func (exasolState *ExasolPersonalState) SetWorkflowState(anyState any) error {
	switch state := anyState.(type) {
	case *WorkflowStateInitialized:
		exasolState.CurrentWorkflowState = WorkflowState{Initialized: state}
	case *WorkflowStateOperationInProgress:
		exasolState.CurrentWorkflowState = WorkflowState{OperationInProgress: state}
	case *WorkflowStateInterrupted:
		exasolState.CurrentWorkflowState = WorkflowState{Interrupted: state}
	case *WorkflowStateDeploymentFailed:
		exasolState.CurrentWorkflowState = WorkflowState{DeploymentFailed: state}
	case *WorkflowStateRunning:
		exasolState.CurrentWorkflowState = WorkflowState{Running: state}
	case *WorkflowStateStopped:
		exasolState.CurrentWorkflowState = WorkflowState{Stopped: state}
	default:
		panic("invalid workflow state")
	}

	return nil
}

func (exasolState *ExasolPersonalState) SetWorkflowStateAndWrite(
	anyState any,
	deploymentDir string,
) error {
	err := exasolState.SetWorkflowState(anyState)
	if err != nil {
		return err
	}

	err = WriteExasolPersonalState(exasolState, deploymentDir)
	if err != nil {
		return err
	}

	return nil
}

var ErrNoWorkflowStateSet = errors.New("no workflow state is set in the workflow state file")

// GetWorkflowState returns the deployment state of the deployment directory.
func (exasolState *ExasolPersonalState) GetWorkflowState() (any, error) {
	if exasolState.CurrentWorkflowState.Initialized != nil {
		return exasolState.CurrentWorkflowState.Initialized, nil
	}
	if exasolState.CurrentWorkflowState.OperationInProgress != nil {
		return exasolState.CurrentWorkflowState.OperationInProgress, nil
	}
	if exasolState.CurrentWorkflowState.Running != nil {
		return exasolState.CurrentWorkflowState.Running, nil
	}
	if exasolState.CurrentWorkflowState.Stopped != nil {
		return exasolState.CurrentWorkflowState.Stopped, nil
	}
	if exasolState.CurrentWorkflowState.Interrupted != nil {
		return exasolState.CurrentWorkflowState.Interrupted, nil
	}
	if exasolState.CurrentWorkflowState.DeploymentFailed != nil {
		return exasolState.CurrentWorkflowState.DeploymentFailed, nil
	}

	return "", ErrNoWorkflowStateSet
}
