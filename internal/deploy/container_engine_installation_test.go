// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
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

// engineRuntimeStub is a runtime on a given engine whose preparation returns
// prepareErr and keeps the options it was given.
type engineRuntimeStub struct {
	*endpointRuntimeStub

	engine     localinstall.ContainerEngine
	prepareErr error
	prepared   []localruntime.PrepareOptions
}

func (stub *engineRuntimeStub) ContainerEngine() localinstall.ContainerEngine {
	return stub.engine
}

func (stub *engineRuntimeStub) Prepare(
	_ context.Context, _, _ io.Writer, options localruntime.PrepareOptions,
) error {
	stub.prepared = append(stub.prepared, options)

	return stub.prepareErr
}

type engineOfferFixture struct {
	*engineChoiceFixture

	unusable *engineRuntimeStub
	other    *engineRuntimeStub
	offers   []localruntime.ContainerEngineOffer
}

var errEngineNotReady = fmt.Errorf(
	"%w: docker is not available: the daemon does not answer",
	localruntime.ErrContainerEngineNotReady,
)

// newEngineOfferFixture prepares a deployment whose automatically chosen
// Docker fails its readiness check on Windows, with Podman as the other
// engine.
func newEngineOfferFixture(
	t *testing.T, installed ...localinstall.ContainerEngine,
) *engineOfferFixture {
	t.Helper()
	fixture := &engineOfferFixture{
		engineChoiceFixture: newEngineChoiceFixture(t, localWindowsOS, "", installed...),
		unusable: &engineRuntimeStub{
			endpointRuntimeStub: &endpointRuntimeStub{},
			engine:              localinstall.EngineDocker,
			prepareErr:          errEngineNotReady,
		},
		other: &engineRuntimeStub{
			endpointRuntimeStub: &endpointRuntimeStub{},
			engine:              localinstall.EnginePodman,
		},
	}
	fixture.backend.runtime = fixture.unusable
	fixture.backend.newRuntime = func(context.Context) (localruntime.Runtime, error) {
		return fixture.other, nil
	}
	fixture.backend.engineCommandFound = fixture.backend.engineInstalled
	fixture.backend.engineUsable = func(context.Context, localinstall.ContainerEngine) bool {
		return true
	}
	fixture.backend.engineInstallOptions = func(
		engines []localinstall.ContainerEngine,
	) ([]localruntime.ContainerEngineOption, error) {
		return []localruntime.ContainerEngineOption{{
			Engine:   engines[0],
			Commands: []localruntime.HostCommand{{Name: "winget", Args: []string{"install"}}},
		}}, nil
	}

	return fixture
}

func (fixture *engineOfferFixture) offer(accept bool) localruntime.PrepareOptions {
	return localruntime.PrepareOptions{
		OfferContainerEngine: func(
			_ context.Context, offer localruntime.ContainerEngineOffer,
		) (bool, error) {
			fixture.offers = append(fixture.offers, offer)

			return accept, nil
		},
	}
}

func (fixture *engineOfferFixture) prepare(options localruntime.PrepareOptions) error {
	return fixture.backend.offerOtherContainerEngine(
		context.Background(), nil, nil, options, fixture.unusable, errEngineNotReady,
	)
}

func TestOfferContainerEngine_AcceptedOfferInstallsSavesAndPreparesTheOtherEngine(t *testing.T) {
	t.Parallel()
	fixture := newEngineOfferFixture(t, localinstall.EngineDocker)

	if err := fixture.prepare(fixture.offer(true)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fixture.offers) != 1 {
		t.Fatalf("expected one offer, got %+v", fixture.offers)
	}
	offer := fixture.offers[0]
	if offer.Unusable != localinstall.EngineDocker ||
		offer.Offer.Engine != localinstall.EnginePodman ||
		!errors.Is(offer.Cause, errEngineNotReady) ||
		len(offer.Offer.Commands) != 1 {
		t.Errorf("expected Podman with its installation offered for Docker, got %+v", offer)
	}
	if saved := fixture.savedEngine(t); saved != "podman" {
		t.Errorf("expected podman to be saved, got %q", saved)
	}
	if len(fixture.other.prepared) != 1 {
		t.Fatalf("expected Podman to be prepared once, got %d", len(fixture.other.prepared))
	}
	approved, err := fixture.other.prepared[0].ApproveHostChange(
		context.Background(), localruntime.HostChangeRequest{
			Kind:   localruntime.HostChangeInstallContainerRuntime,
			Engine: localinstall.EnginePodman,
		},
	)
	if err != nil || !approved {
		t.Errorf("expected accepting the offer to approve installing Podman, got %v %v",
			approved, err)
	}
}

func TestOfferContainerEngine_InstalledOtherEngineIsOfferedWithoutInstallation(t *testing.T) {
	t.Parallel()
	fixture := newEngineOfferFixture(
		t, localWindowsOS, localinstall.EngineDocker, localinstall.EnginePodman,
	)

	if err := fixture.prepare(fixture.offer(true)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fixture.offers) != 1 || len(fixture.offers[0].Offer.Commands) != 0 {
		t.Errorf("expected an offer without installation, got %+v", fixture.offers)
	}
}

func TestOfferContainerEngine_DeclinedOrUnattendedRunsNameTheOtherEngine(t *testing.T) {
	t.Parallel()
	for name, accept := range map[string]*bool{"declined": new(bool), "unattended": nil} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fixture := newEngineOfferFixture(t, localinstall.EngineDocker)
			options := localruntime.PrepareOptions{}
			if accept != nil {
				options = fixture.offer(*accept)
			}

			err := fixture.prepare(options)

			if !errors.Is(err, errEngineNotReady) ||
				!strings.Contains(err.Error(), "`exasol config set --container-engine podman`") {
				t.Fatalf("expected the cause and how to select Podman, got %v", err)
			}
			if saved := fixture.savedEngine(t); saved != "" {
				t.Errorf("expected nothing to be saved, got %q", saved)
			}
			if len(fixture.other.prepared) != 0 {
				t.Error("expected Podman not to be prepared")
			}
		})
	}
}

func TestOfferContainerEngine_OffersOnlyAnEngineThatCanRun(t *testing.T) {
	t.Parallel()
	tests := map[string]func(*engineOfferFixture){
		// Linux installs neither engine.
		"missing on Linux": func(fixture *engineOfferFixture) {
			fixture.backend.goos = localLinuxOS
		},
		"winget unavailable": func(fixture *engineOfferFixture) {
			fixture.backend.engineInstallOptions = func(
				[]localinstall.ContainerEngine,
			) ([]localruntime.ContainerEngineOption, error) {
				return nil, errors.New("winget is not available")
			}
		},
	}
	for name, arrange := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fixture := newEngineOfferFixture(t, localinstall.EngineDocker)
			arrange(fixture)

			err := fixture.prepare(fixture.offer(true))

			if len(fixture.offers) != 0 {
				t.Errorf("expected no offer, got %+v", fixture.offers)
			}
			if !strings.Contains(err.Error(), "--container-engine podman") {
				t.Errorf("expected how to select Podman, got %v", err)
			}
		})
	}
}

// A failure of the launcher's own files would follow the deployment to any
// engine, so it is returned as it is.
func TestOfferContainerEngine_FailureOutsideTheEngineIsNotOffered(t *testing.T) {
	t.Parallel()
	fixture := newEngineOfferFixture(t, localinstall.EngineDocker)
	filesystemErr := errors.New("failed to create local runtime directory: permission denied")

	err := fixture.backend.offerOtherContainerEngine(
		context.Background(), nil, nil, fixture.offer(true), fixture.unusable, filesystemErr,
	)

	if len(fixture.offers) != 0 || !errors.Is(err, filesystemErr) ||
		strings.Contains(err.Error(), "--container-engine") {
		t.Errorf("expected the filesystem error alone and no offer, got %v and %+v",
			err, fixture.offers)
	}
	if fixture.savedEngine(t) != "" || len(fixture.other.prepared) != 0 {
		t.Error("expected the deployment to keep its engine")
	}
}

// Selecting an engine that cannot run either would only trade one failure for
// another, so the error does not suggest it.
func TestOfferContainerEngine_DoesNotSuggestAnUnusableOtherEngine(t *testing.T) {
	t.Parallel()
	fixture := newEngineOfferFixture(t, localinstall.EngineDocker)
	fixture.backend.engineCommandFound = func(context.Context, localinstall.ContainerEngine) bool {
		return true
	}
	fixture.backend.engineUsable = func(context.Context, localinstall.ContainerEngine) bool {
		return false
	}

	err := fixture.prepare(fixture.offer(true))

	if len(fixture.offers) != 0 {
		t.Errorf("expected no offer, got %+v", fixture.offers)
	}
	if !errors.Is(err, errEngineNotReady) || strings.Contains(err.Error(), "--container-engine") {
		t.Errorf("expected only Docker's error, got %v", err)
	}
}

func TestOfferContainerEngine_MissingChosenEngineKeepsItsError(t *testing.T) {
	t.Parallel()
	fixture := newEngineOfferFixture(t)

	err := fixture.prepare(fixture.offer(true))

	if !errors.Is(err, errEngineNotReady) || len(fixture.offers) != 0 ||
		strings.Contains(err.Error(), "--container-engine") {
		t.Errorf("expected the error unchanged without an offer, got %v %+v", err, fixture.offers)
	}
}

func TestChoosesContainerEngineAutomatically_OnlyForAutoBeforeTheFirstStart(t *testing.T) {
	t.Parallel()
	if !newEngineChoiceFixture(t, localWindowsOS, "auto").backend.
		choosesContainerEngineAutomatically() {
		t.Error("expected auto to choose before the first start")
	}
	if newEngineChoiceFixture(t, localWindowsOS, "docker").backend.
		choosesContainerEngineAutomatically() {
		t.Error("expected an explicit engine to be kept")
	}
	recorded := newEngineChoiceFixture(t, localLinuxOS, "")
	dataDir := filepath.Join(recorded.deployment.Resolve("local"), "runtime", "exa")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("create database data: %v", err)
	}
	if recorded.backend.choosesContainerEngineAutomatically() {
		t.Error("expected a started deployment to keep its engine")
	}
}

func TestReportSkippedDocker_ExplainsWhyPodmanIsUsed(t *testing.T) {
	t.Parallel()
	fixture := newEngineOfferFixture(t)
	fixture.backend.dockerRunsWindows = func(context.Context) bool { return true }
	var progress bytes.Buffer

	fixture.backend.reportSkippedDocker(
		context.Background(), localruntime.PrepareOptions{Progress: &progress}, fixture.other,
	)

	if !strings.Contains(progress.String(), "Docker runs Windows containers") {
		t.Errorf("expected the reason Docker is skipped, got %q", progress.String())
	}
}
