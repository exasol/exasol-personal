// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package config

import (
	"errors"
	"testing"
)

func TestReadLocalDeploymentInfo_RoundTrip(t *testing.T) {
	t.Parallel()

	// Given
	deploymentDir := t.TempDir()
	info := &LocalDeploymentInfo{
		Backend:         DeploymentBackendLocal,
		DeploymentID:    "local-test",
		DeploymentState: "running",
		ClusterSize:     1,
		ClusterState:    "running",
		Local: &LocalDeploymentRuntime{
			Host:                       "127.0.0.1",
			DBPort:                     8563,
			UIPort:                     8443,
			Username:                   "sys",
			InsecureSkipCertValidation: true,
		},
	}

	// When
	if err := WriteLocalDeploymentInfo(deploymentDir, info); err != nil {
		t.Fatalf("failed to write local deployment info: %v", err)
	}
	actual, err := ReadLocalDeploymentInfo(deploymentDir)

	// Then
	if err != nil {
		t.Fatalf("failed to read local deployment info: %v", err)
	}
	if actual.Backend != DeploymentBackendLocal {
		t.Fatalf("expected backend %q, got %q", DeploymentBackendLocal, actual.Backend)
	}
	if actual.Local == nil || actual.Local.DBPort != 8563 || actual.Local.UIPort != 8443 {
		t.Fatalf("unexpected local runtime payload: %#v", actual.Local)
	}
}

func TestReadLocalDeploymentInfo_RejectsCloudStyleDeploymentJSON(t *testing.T) {
	t.Parallel()

	// Given
	deploymentDir := t.TempDir()
	nodeDetails := &NodeDetails{
		DeploymentId: "cloud-test",
		ClusterSize:  1,
		Nodes:        map[string]nodeDetailsNode{},
	}
	if err := writeConfig(nodeDetails, deploymentDir+"/"+nodeDetailsFileName, "node details"); err != nil {
		t.Fatalf("failed to write node details: %v", err)
	}

	// When
	_, err := ReadLocalDeploymentInfo(deploymentDir)

	// Then
	if !errors.Is(err, ErrNotLocalDeploymentInfo) {
		t.Fatalf("expected ErrNotLocalDeploymentInfo, got %v", err)
	}
}
