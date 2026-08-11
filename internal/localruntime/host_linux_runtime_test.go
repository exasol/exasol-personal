// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
)

func TestLinuxHostPodmanStartConfig_UsesReferenceDefaults(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	localRuntime := NewHostLinuxRuntime(deployment, nil)

	// When
	startConfig, err := localRuntime.podmanStartConfig(RuntimeConfig{})
	// Then
	if err != nil {
		t.Fatalf("expected default config, got %v", err)
	}
	if startConfig.ContainerDBPort != nanoDBPort {
		t.Fatalf("unexpected default DB port: %#v", startConfig)
	}
	if startConfig.DataDir != filepath.Join(localRuntime.paths.WorkDir, nanoDataDirName) {
		t.Fatalf("unexpected Nano data directory %q", startConfig.DataDir)
	}
	if len(startConfig.InitParams) != 1 ||
		startConfig.InitParams[0] != "maxConnectionsLicenseLimit=20" {
		t.Fatalf("unexpected Nano init params: %#v", startConfig.InitParams)
	}
}

func TestLinuxHostPodmanStartConfig_UsesOnlyCommonRuntimeConfig(t *testing.T) {
	t.Parallel()

	// Given
	localRuntime := NewHostLinuxRuntime(config.NewDeploymentDir(t.TempDir()), nil)
	vmConfig := VMConfig{
		RuntimeConfig: RuntimeConfig{Ports: "ssh:20022, db:28563, ui:28443"},
		CPUCount:      32,
		MemoryMB:      131072,
		DataSizeGB:    4096,
	}

	// When
	startConfig, err := localRuntime.podmanStartConfig(vmConfig.RuntimeConfig)
	// Then
	if err != nil {
		t.Fatalf("expected host port override, got %v", err)
	}
	if startConfig.ContainerDBPort != 28563 {
		t.Fatalf("expected published DB port override, got %#v", startConfig)
	}
}

func TestLinuxHostReadEndpoint_ReturnsPublishedDatabasePort(t *testing.T) {
	t.Parallel()

	// Given
	localRuntime := NewHostLinuxRuntime(config.NewDeploymentDir(t.TempDir()), nil)
	localRuntime.endpoint = &RuntimeEndpoint{DBPort: 28563}

	// When
	endpoint, err := localRuntime.ReadEndpoints()
	// Then
	if err != nil {
		t.Fatalf("expected endpoint, got %v", err)
	}
	if endpoint.DBPort != 28563 {
		t.Fatalf("expected published DB port 28563, got %#v", endpoint)
	}
	if endpoint.SSHPort != 0 || endpoint.PrivateKeyRelativePath != "" {
		t.Fatalf("expected no VM-only endpoint details, got %#v", endpoint)
	}
}

func TestLinuxHostReadEndpoint_RejectsReadBeforeStart(t *testing.T) {
	t.Parallel()

	// Given
	localRuntime := NewHostLinuxRuntime(config.NewDeploymentDir(t.TempDir()), nil)

	// When
	_, err := localRuntime.ReadEndpoints()
	// Then
	if err == nil || !strings.Contains(err.Error(), "before start") {
		t.Fatalf("expected endpoint availability error, got %v", err)
	}
}

func TestResolveLinuxHostDBPort_RejectsInvalidMappings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		ports string
	}{
		{name: "missing separator", ports: "db"},
		{name: "missing service", ports: ":8563"},
		{name: "missing port", ports: "db:"},
		{name: "non numeric", ports: "db:abc"},
		{name: "zero", ports: "db:0"},
		{name: "too large", ports: "db:65536"},
		{name: "duplicate DB", ports: "db:8563,db:28563"},
		{name: "malformed ignored service", ports: "ssh:not-a-port,db:8563"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// Given / When
			_, err := resolveLinuxHostDBPort(test.ports)

			// Then
			if err == nil {
				t.Fatalf("expected invalid mapping error for %q", test.ports)
			}
			if !strings.Contains(err.Error(), "local") {
				t.Fatalf("expected contextual local port error, got %v", err)
			}
		})
	}
}
