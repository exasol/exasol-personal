// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package config

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

const DeploymentBackendLocal = "local"

var ErrNotLocalDeploymentInfo = errors.New("deployment info is not for a local deployment")

type LocalDeploymentInfo struct {
	Backend         string                  `json:"backend"`
	DeploymentID    string                  `json:"deploymentId"`
	DeploymentState string                  `json:"deploymentState,omitempty"`
	ClusterSize     int                     `json:"clusterSize,omitempty"`
	ClusterState    string                  `json:"clusterState,omitempty"`
	Local           *LocalDeploymentRuntime `json:"local,omitempty"`
}

type LocalDeploymentRuntime struct {
	Host                       string `json:"host"`
	DBPort                     int    `json:"dbPort"`
	UIPort                     int    `json:"uiPort"`
	Username                   string `json:"username,omitempty"`
	CertFingerprint            string `json:"certFingerprint,omitempty"`
	InsecureSkipCertValidation bool   `json:"insecureSkipCertValidation,omitempty"`
	RuntimeRoot                string `json:"runtimeRoot,omitempty"`
	ControlSocketPath          string `json:"controlSocketPath,omitempty"`
	RuntimeStatePath           string `json:"runtimeStatePath,omitempty"`
	PIDFilePath                string `json:"pidFilePath,omitempty"`
	ConsoleLogPath             string `json:"consoleLogPath,omitempty"`
	RunnerLogPath              string `json:"runnerLogPath,omitempty"`
}

func ReadLocalDeploymentInfo(deploymentDir string) (*LocalDeploymentInfo, error) {
	filepath, exists, err := GetDeploymentInfoFilePath(deploymentDir)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf(
			"%w: failed to open local deployment info file %q",
			ErrMissingConfigFile,
			filepath,
		)
	}

	info, err := readConfig[LocalDeploymentInfo](filepath, "local deployment info")
	if err != nil {
		return nil, err
	}

	if !info.IsLocal() {
		return nil, ErrNotLocalDeploymentInfo
	}

	return info, nil
}

func WriteLocalDeploymentInfo(deploymentDir string, info *LocalDeploymentInfo) error {
	if info == nil {
		return errors.New("local deployment info is required")
	}
	if !info.IsLocal() {
		return fmt.Errorf("%w: backend %q", ErrNotLocalDeploymentInfo, info.Backend)
	}

	path := filepath.Join(deploymentDir, nodeDetailsFileName)

	return writeConfig(info, path, "local deployment info")
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
