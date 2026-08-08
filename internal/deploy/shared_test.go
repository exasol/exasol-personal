// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/exasol/exasol-personal/internal/config"
)

func TestGetn11DetailsReturnsErrNoNodesFoundWhenDeploymentHasNoNodes(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())
	if err := config.WriteDeploymentInfo(deployment.Root(), &config.DeploymentInfo{
		Backend:      "tofu",
		DeploymentId: "dep-1",
	}); err != nil {
		t.Fatalf("failed to write deployment info: %v", err)
	}

	_, err := Getn11Details(deployment)

	if !errors.Is(err, ErrNoNodesFound) {
		t.Fatalf("expected ErrNoNodesFound, got %v", err)
	}
}

func TestGetn11DetailsResolvesSSHDetailsForFirstNode(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())
	writeTestSSHKeyFile(t, deployment, "keys/n11.pem")
	if err := config.WriteDeploymentInfo(deployment.Root(), &config.DeploymentInfo{
		Backend:      "tofu",
		DeploymentId: "dep-1",
		Nodes: map[string]config.DeploymentNode{
			"n11": {
				PublicIp: "203.0.113.10",
				Ssh: config.DeploymentSSH{
					Username: "sys",
					Port:     "22",
					KeyFile:  "keys/n11.pem",
				},
			},
		},
	}); err != nil {
		t.Fatalf("failed to write deployment info: %v", err)
	}

	details, err := Getn11Details(deployment)
	if err != nil {
		t.Fatalf("expected SSH details to resolve, got %v", err)
	}
	if details.Host != "203.0.113.10" || details.User != "sys" || details.Port != "22" {
		t.Fatalf("unexpected SSH details: %+v", details)
	}
}

func TestPollWithBackoffReturnsImmediatelyWhenConditionIsAlreadyMet(t *testing.T) {
	t.Parallel()

	calls := 0

	err := PollWithBackoff(context.Background(), func(context.Context) (bool, error) {
		calls++

		return true, nil
	}, WaitParams{InitialBackoff: 1, MaxBackoff: 1})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected exactly one condition check, got %d", calls)
	}
}

func TestPollWithBackoffRetriesUntilConditionSucceeds(t *testing.T) {
	t.Parallel()

	calls := 0

	err := PollWithBackoff(context.Background(), func(context.Context) (bool, error) {
		calls++

		return calls >= 2, nil
	}, WaitParams{InitialBackoff: 1, MaxBackoff: 1})
	if err != nil {
		t.Fatalf("expected eventual success, got %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected exactly two condition checks, got %d", calls)
	}
}

func TestPollWithBackoffReturnsLastConditionErrorOnTimeout(t *testing.T) {
	t.Parallel()

	persistentErr := errors.New("database not ready yet")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := PollWithBackoff(ctx, func(context.Context) (bool, error) {
		return false, persistentErr
	}, WaitParams{InitialBackoff: 1, MaxBackoff: 1})

	if !errors.Is(err, persistentErr) {
		t.Fatalf("expected the last condition error to surface, got %v", err)
	}
}

func TestPollWithBackoffReturnsContextErrorWhenConditionNeverErrors(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := PollWithBackoff(ctx, func(context.Context) (bool, error) {
		return false, nil
	}, WaitParams{InitialBackoff: 1, MaxBackoff: 1})

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got %v", err)
	}
}
