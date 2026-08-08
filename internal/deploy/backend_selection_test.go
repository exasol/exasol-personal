// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"os"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/presets"
)

func newTofuTestDeployment(t *testing.T) config.DeploymentDir {
	t.Helper()

	deployment := config.NewDeploymentDir(t.TempDir())
	if err := os.MkdirAll(deployment.InfrastructureDir(), 0o700); err != nil {
		t.Fatalf("create infrastructure dir failed: %v", err)
	}
	writeTestFile(t, deployment.InfrastructureManifestPath(), `
name: Test Tofu Infrastructure
description: test infrastructure
backend: tofu
tofu: {}
`)

	return deployment
}

// resolveBackendKind and newDeploymentBackend (given a manifest) are already
// covered in backend_test.go; the tests below add newDeploymentBackendForDeployment
// (reading the manifest from disk) and readInfrastructurePresetConfigVariables,
// which were not covered anywhere.

func TestNewDeploymentBackendForDeployment_ResolvesLocalBackend(t *testing.T) {
	t.Parallel()

	deployment := newLocalTestDeployment(t)

	backend, err := newDeploymentBackendForDeployment(deployment)
	if err != nil {
		t.Fatalf("expected a local backend to resolve, got %v", err)
	}
	if backend == nil {
		t.Fatal("expected a non-nil backend")
	}
}

func TestNewDeploymentBackendForDeployment_ResolvesTofuBackend(t *testing.T) {
	t.Parallel()

	deployment := newTofuTestDeployment(t)

	backend, err := newDeploymentBackendForDeployment(deployment)
	if err != nil {
		t.Fatalf("expected a tofu backend to resolve, got %v", err)
	}
	if backend == nil {
		t.Fatal("expected a non-nil backend")
	}
}

func TestNewDeploymentBackendForDeployment_MissingManifestReturnsError(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())

	_, err := newDeploymentBackendForDeployment(deployment)
	if err == nil {
		t.Fatal("expected an error when the infrastructure manifest is missing")
	}
}

func TestReadInfrastructurePresetConfigVariables_TofuWithoutTofuConfigIsEmpty(t *testing.T) {
	t.Parallel()

	manifest := &presets.InfrastructureManifest{Backend: "tofu"}

	variables, err := readInfrastructurePresetConfigVariables(PresetRef{Name: "aws"}, manifest)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(variables) != 0 {
		t.Fatalf("expected an empty variable set, got %+v", variables)
	}
}

func TestReadInfrastructurePresetConfigVariables_LocalBackendUsesLocalDefinitions(t *testing.T) {
	t.Parallel()

	manifest := &presets.InfrastructureManifest{
		Backend: "local",
		Local:   &presets.InfrastructureLocal{},
	}

	variables, err := readInfrastructurePresetConfigVariables(PresetRef{Name: "local"}, manifest)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(variables) == 0 {
		t.Fatal("expected local backend to declare configurable variables")
	}
}
