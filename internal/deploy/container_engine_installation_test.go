// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localinstall"
	"github.com/exasol/exasol-personal/internal/localruntime"
	"github.com/exasol/exasol-personal/internal/presets"
)

type engineChoiceFixture struct {
	backend    *localBackend
	deployment config.DeploymentDir
	asked      []localruntime.ContainerEngineChoiceRequest
}

func newEngineChoiceFixture(
	t *testing.T,
	goos, configured string,
	installed ...localinstall.ContainerEngine,
) *engineChoiceFixture {
	t.Helper()
	deployment := config.NewDeploymentDir(t.TempDir())
	if err := os.MkdirAll(deployment.InfrastructureDir(), 0o700); err != nil {
		t.Fatalf("create infrastructure dir: %v", err)
	}
	manifest := &presets.InfrastructureManifest{
		Backend: backendTypeLocal,
		Local:   &presets.InfrastructureLocal{ContainerEngine: configured},
	}
	backend := newLocalBackendForPlatform(deployment, manifest, nil, goos, "amd64")
	backend.engineInstalled = func(_ context.Context, engine localinstall.ContainerEngine) bool {
		return slices.Contains(installed, engine)
	}
	backend.engineCommandFound = func(context.Context, localinstall.ContainerEngine) bool {
		return false
	}
	backend.engineInstallOptions = func(
		engines []localinstall.ContainerEngine,
	) ([]localruntime.ContainerEngineOption, error) {
		options := make([]localruntime.ContainerEngineOption, 0, len(engines))
		for _, engine := range engines {
			options = append(options, localruntime.ContainerEngineOption{Engine: engine})
		}

		return options, nil
	}

	return &engineChoiceFixture{backend: backend, deployment: deployment}
}

func (fixture *engineChoiceFixture) options(
	answer localinstall.ContainerEngine,
) localruntime.PrepareOptions {
	return localruntime.PrepareOptions{
		ChooseContainerEngine: func(
			_ context.Context, request localruntime.ContainerEngineChoiceRequest,
		) (localinstall.ContainerEngine, error) {
			fixture.asked = append(fixture.asked, request)

			return answer, nil
		},
	}
}

func (fixture *engineChoiceFixture) savedEngine(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(fixture.deployment.InfrastructureManifestPath())
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		if value, found := strings.CutPrefix(strings.TrimSpace(line), "containerEngine:"); found {
			return strings.TrimSpace(value)
		}
	}

	return ""
}

func TestChooseContainerEngine_SavesTheChosenEngineAndApprovesItsInstallation(t *testing.T) {
	t.Parallel()
	fixture := newEngineChoiceFixture(t, localWindowsOS, "")

	options, err := fixture.backend.chooseContainerEngineToInstall(
		context.Background(), fixture.options(localinstall.EngineDocker),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fixture.asked) != 1 || len(fixture.asked[0].Options) != 2 ||
		fixture.asked[0].Options[0].Engine != localinstall.EnginePodman {
		t.Fatalf("expected one choice listing Podman first, got %+v", fixture.asked)
	}
	if saved := fixture.savedEngine(t); saved != "docker" {
		t.Errorf("expected docker to be saved, got %q", saved)
	}
	approved, err := options.ApproveHostChange(context.Background(), localruntime.HostChangeRequest{
		Kind:   localruntime.HostChangeInstallContainerRuntime,
		Engine: localinstall.EngineDocker,
	})
	if err != nil || !approved {
		t.Errorf("expected the choice to approve installing Docker, got %v %v", approved, err)
	}
}

func TestChooseContainerEngine_CancelledChoiceSavesNothing(t *testing.T) {
	t.Parallel()
	fixture := newEngineChoiceFixture(t, localWindowsOS, "auto")

	_, err := fixture.backend.chooseContainerEngineToInstall(
		context.Background(), fixture.options(""),
	)

	if err == nil || !strings.Contains(err.Error(), "--container-engine") {
		t.Fatalf("expected a failure naming --container-engine, got %v", err)
	}
	if saved := fixture.savedEngine(t); saved != "" {
		t.Errorf("expected nothing to be saved, got %q", saved)
	}
}

func TestChooseContainerEngine_AsksOnlyWhenAutoFindsNoEngineOnWindows(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		goos       string
		configured string
		installed  []localinstall.ContainerEngine
	}{
		{name: "Linux", goos: localLinuxOS},
		{name: "explicit engine", goos: localWindowsOS, configured: "docker"},
		{
			name: "Docker installed", goos: localWindowsOS,
			installed: []localinstall.ContainerEngine{localinstall.EngineDocker},
		},
		{
			name: "Podman installed", goos: localWindowsOS,
			installed: []localinstall.ContainerEngine{localinstall.EnginePodman},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fixture := newEngineChoiceFixture(t, test.goos, test.configured, test.installed...)

			_, err := fixture.backend.chooseContainerEngineToInstall(
				context.Background(), fixture.options(localinstall.EngineDocker),
			)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(fixture.asked) != 0 {
				t.Errorf("expected no choice, got %+v", fixture.asked)
			}
		})
	}
}

// A Docker that runs Windows containers is installed although `auto` passes
// over it, so the deployment keeps the Podman path instead of offering to
// install Docker Desktop.
func TestChooseContainerEngine_DoesNotOfferAnInstalledDocker(t *testing.T) {
	t.Parallel()
	fixture := newEngineChoiceFixture(t, localWindowsOS, "")
	fixture.backend.engineCommandFound = func(
		_ context.Context, engine localinstall.ContainerEngine,
	) bool {
		return engine == localinstall.EngineDocker
	}

	_, err := fixture.backend.chooseContainerEngineToInstall(
		context.Background(), fixture.options(localinstall.EngineDocker),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fixture.asked) != 0 {
		t.Errorf("expected no choice while Docker is installed, got %+v", fixture.asked)
	}
}

func TestChooseContainerEngine_KeepsTheRecordedEngine(t *testing.T) {
	t.Parallel()
	fixture := newEngineChoiceFixture(t, localWindowsOS, "")
	// Database data without an engine record belongs to a Podman deployment.
	dataDir := filepath.Join(fixture.deployment.Resolve("local"), "runtime", "exa")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("create database data: %v", err)
	}

	_, err := fixture.backend.chooseContainerEngineToInstall(
		context.Background(), fixture.options(localinstall.EngineDocker),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fixture.asked) != 0 {
		t.Errorf("expected no choice for a deployment with an engine, got %+v", fixture.asked)
	}
}
