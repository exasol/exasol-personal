// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package config

import (
	"encoding/json"
	"errors"
	"os"
)

type sidecarState struct {
	Failures map[string]string `json:"failures,omitempty"`
}

func (d DeploymentDir) SidecarStatePath() string {
	return d.Resolve("sidecars-state.json")
}

func loadSidecarState(deployment DeploymentDir) (sidecarState, error) {
	data, err := os.ReadFile(deployment.SidecarStatePath())
	if errors.Is(err, os.ErrNotExist) {
		return sidecarState{}, nil
	}
	if err != nil {
		return sidecarState{}, err
	}
	var state sidecarState
	if err := json.Unmarshal(data, &state); err != nil {
		return sidecarState{}, err
	}

	return state, nil
}

func storeSidecarState(deployment DeploymentDir, state sidecarState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}

	return WriteFileAtomically(deployment.SidecarStatePath(), data, sidecarsFileMode)
}

func ReadSidecarState(deployment DeploymentDir) (map[string]string, error) {
	state, err := loadSidecarState(deployment)
	if err != nil {
		return nil, err
	}
	if state.Failures == nil {
		return map[string]string{}, nil
	}

	return state.Failures, nil
}

// WriteSidecarState accepts redacted diagnostics while the deployment lock is held.
func WriteSidecarState(deployment DeploymentDir, failures map[string]string) error {
	state, err := loadSidecarState(deployment)
	if err != nil {
		return err
	}
	state.Failures = failures

	return storeSidecarState(deployment, state)
}
