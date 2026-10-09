// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package config

import (
	"encoding/json"
	"errors"
	"os"
)

func (d DeploymentDir) SidecarStatePath() string {
	return d.Resolve("sidecars-state.json")
}

func ReadSidecarState(deployment DeploymentDir) (map[string]string, error) {
	data, err := os.ReadFile(deployment.SidecarStatePath())
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	var failures map[string]string
	if err := json.Unmarshal(data, &failures); err != nil {
		return nil, err
	}
	if failures == nil {
		failures = map[string]string{}
	}

	return failures, nil
}

// WriteSidecarState accepts redacted diagnostics while the deployment lock is held.
func WriteSidecarState(deployment DeploymentDir, failures map[string]string) error {
	data, err := json.Marshal(failures)
	if err != nil {
		return err
	}

	return writeSidecarFile(deployment, deployment.SidecarStatePath(), data)
}
