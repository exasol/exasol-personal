// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/connect"
	"github.com/exasol/exasol-personal/internal/presets"
)

func TestConnect_BlockedStateReturnsErrorWithoutResolvingConnectionInfo(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())
	if err := InitDeployment(
		context.Background(),
		PresetRef{Name: presets.DefaultInfrastructure},
		PresetRef{Name: presets.DefaultInstallation},
		map[string]string{},
		map[string]string{},
		deployment,
		false,
		"0.0.0",
	); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	err := Connect(context.Background(), &connect.Opts{}, deployment)
	if !errors.Is(err, ErrUnexpectedDeploymentStatus) {
		t.Fatalf("expected ErrUnexpectedDeploymentStatus, got %v", err)
	}
}

func TestConnect_MissingStateReturnsError(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())

	if err := Connect(context.Background(), &connect.Opts{}, deployment); err == nil {
		t.Fatal("expected an error when no launcher state has been persisted")
	}
}
