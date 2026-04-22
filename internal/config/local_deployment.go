// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package config

import (
	"errors"
	"fmt"
	"strings"
)

var ErrNotLocalDeploymentInfo = errors.New("deployment info is not for a local deployment")

type LocalDeploymentInfo struct {
	Backend         string                  `json:"backend"`
	DeploymentID    string                  `json:"deploymentId"`
	DeploymentState string                  `json:"deploymentState,omitempty"`
	ClusterSize     int                     `json:"clusterSize,omitempty"`
	ClusterState    string                  `json:"clusterState,omitempty"`
	Local           *LocalDeploymentRuntime `json:"local,omitempty"`
}

type LocalDeploymentRuntime = DeploymentRuntime

func ReadLocalDeploymentInfo(deploymentDir string) (*LocalDeploymentInfo, error) {
	info, err := ReadDeploymentInfo(NewDeploymentDir(deploymentDir))
	if err != nil {
		return nil, err
	}
	if info.Runtime == nil ||
		!strings.EqualFold(strings.TrimSpace(info.Backend), DeploymentBackendLocal) {
		return nil, ErrNotLocalDeploymentInfo
	}

	return &LocalDeploymentInfo{
		Backend:         info.Backend,
		DeploymentID:    info.DeploymentId,
		DeploymentState: info.DeploymentState,
		ClusterSize:     info.ClusterSize,
		ClusterState:    info.ClusterState,
		Local:           info.Runtime,
	}, nil
}

func WriteLocalDeploymentInfo(deploymentDir string, info *LocalDeploymentInfo) error {
	if info == nil {
		return errors.New("local deployment info is required")
	}
	if !info.IsLocal() {
		return fmt.Errorf("%w: backend %q", ErrNotLocalDeploymentInfo, info.Backend)
	}

	deploymentInfo := &DeploymentInfo{
		Backend:         DeploymentBackendLocal,
		DeploymentId:    info.DeploymentID,
		DeploymentState: info.DeploymentState,
		ClusterSize:     info.ClusterSize,
		ClusterState:    info.ClusterState,
		Runtime:         info.Local,
	}

	return WriteDeploymentInfo(deploymentDir, deploymentInfo)
}

func (i *LocalDeploymentInfo) IsLocal() bool {
	if i == nil {
		return false
	}

	return strings.EqualFold(strings.TrimSpace(i.Backend), DeploymentBackendLocal) && i.Local != nil
}

func GetDeploymentInfoFilePath(deploymentDir string) (string, bool, error) {
	return findExistingFile(deploymentDir, nodeDetailsFileName)
}
