// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localinstall"
)

// destroyRuntimeStub is a Docker runtime that records how it was destroyed.
type destroyRuntimeStub struct {
	*engineRuntimeStub

	destroyed              int
	destroyedWithoutEngine int
}

var errDockerMissing = errors.New(`docker is not available: exec: "docker": not found`)

func (stub *destroyRuntimeStub) Destroy(context.Context, io.Writer, io.Writer) error {
	stub.destroyed++

	return errDockerMissing
}

func (stub *destroyRuntimeStub) DestroyWithoutContainerEngine() error {
	stub.destroyedWithoutEngine++

	return nil
}

type destroyWithoutEngineFixture struct {
	backend *localBackend
	runtime *destroyRuntimeStub
	asked   []MissingContainerEngine
}

// newDestroyWithoutEngineFixture is a Docker deployment whose docker command
// is found only when commandFound is set.
func newDestroyWithoutEngineFixture(
	t *testing.T,
	commandFound bool,
) *destroyWithoutEngineFixture {
	t.Helper()
	choice := newEngineChoiceFixture(t, localLinuxOS, "")
	if err := config.WriteExasolPersonalState(
		&config.ExasolPersonalState{DeploymentId: "2387318e"}, choice.deployment,
	); err != nil {
		t.Fatalf("write state: %v", err)
	}
	if err := os.WriteFile(choice.deployment.SecretsPath(), []byte("{}"), 0o600); err != nil {
		t.Fatalf("write secrets: %v", err)
	}
	choice.backend.engineCommandFound = func(context.Context, localinstall.ContainerEngine) bool {
		return commandFound
	}
	stub := &destroyRuntimeStub{engineRuntimeStub: &engineRuntimeStub{
		endpointRuntimeStub: &endpointRuntimeStub{deployment: choice.deployment},
		engine:              localinstall.EngineDocker,
	}}
	choice.backend.runtime = stub

	return &destroyWithoutEngineFixture{backend: choice.backend, runtime: stub}
}

func (fixture *destroyWithoutEngineFixture) options(accept bool) DestroyOptions {
	return DestroyOptions{
		ConfirmWithoutContainerEngine: func(
			_ context.Context, missing MissingContainerEngine,
		) (bool, error) {
			fixture.asked = append(fixture.asked, missing)

			return accept, nil
		},
	}
}

func (fixture *destroyWithoutEngineFixture) destroy(options DestroyOptions) error {
	return fixture.backend.Destroy(context.Background(), nil, nil, options)
}

func TestLocalBackendDestroy_AcceptedRemovesFilesWithoutTheEngine(t *testing.T) {
	t.Parallel()
	fixture := newDestroyWithoutEngineFixture(t, false)

	if err := fixture.destroy(fixture.options(true)); err != nil {
		t.Fatalf("Destroy() unexpected error: %v", err)
	}
	if len(fixture.asked) != 1 || fixture.asked[0] != (MissingContainerEngine{
		Engine:    localinstall.EngineDocker,
		Container: "exasol-db-2387318e",
		Network:   "exasol-db-2387318e-services",
	}) {
		t.Fatalf("expected one question naming what Docker keeps, got %+v", fixture.asked)
	}
	if fixture.runtime.destroyedWithoutEngine != 1 || fixture.runtime.destroyed != 0 {
		t.Errorf("expected only the engine-free removal, got %d engine-free and %d engine",
			fixture.runtime.destroyedWithoutEngine, fixture.runtime.destroyed)
	}
	secrets := fixture.runtime.deployment.SecretsPath()
	if _, err := os.Stat(secrets); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected %s to be removed, got %v", secrets, err)
	}
}

func TestLocalBackendDestroy_DeclinedFailsAsBefore(t *testing.T) {
	t.Parallel()
	fixture := newDestroyWithoutEngineFixture(t, false)

	err := fixture.destroy(fixture.options(false))

	if !errors.Is(err, errDockerMissing) {
		t.Fatalf("expected the engine's error, got %v", err)
	}
	if fixture.runtime.destroyedWithoutEngine != 0 || fixture.runtime.destroyed != 1 {
		t.Errorf("expected only the engine destroy, got %d engine-free and %d engine",
			fixture.runtime.destroyedWithoutEngine, fixture.runtime.destroyed)
	}
}

func TestLocalBackendDestroy_UnattendedFailsWithoutAsking(t *testing.T) {
	t.Parallel()
	fixture := newDestroyWithoutEngineFixture(t, false)

	err := fixture.destroy(DestroyOptions{})

	if !errors.Is(err, errDockerMissing) || fixture.runtime.destroyedWithoutEngine != 0 {
		t.Fatalf("expected the engine's error and no engine-free removal, got %v", err)
	}
}

func TestLocalBackendDestroy_FoundEngineIsNotAskedAbout(t *testing.T) {
	t.Parallel()
	fixture := newDestroyWithoutEngineFixture(t, true)

	err := fixture.destroy(fixture.options(true))

	if !errors.Is(err, errDockerMissing) || len(fixture.asked) != 0 ||
		fixture.runtime.destroyedWithoutEngine != 0 {
		t.Fatalf("expected the engine destroy without a question, got %v and %+v",
			err, fixture.asked)
	}
}
