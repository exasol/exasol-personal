// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"testing"

	"github.com/exasol/exasol-personal/internal/runtimeadapter"
)

func TestClassifyLocalReachabilityUsesV2ForwardHealth(t *testing.T) {
	// Given
	t.Setenv(localAllowUnsupportedEnv, "1")
	deployment := newLocalTestDeployment(t)
	installLocalRuntimeTestAssets(t)
	writeFakeV2Provider(t, deployment, map[string]any{
		"schemaVersion": 1,
		"phase":         "running",
		"forwards": []map[string]any{
			{"name": "ssh", "hostAddress": "127.0.0.1", "hostPort": 20022, "health": "blocked"},
			{"name": "database", "hostAddress": "127.0.0.1", "hostPort": 8563, "health": "blocked"},
		},
		"hook": map[string]any{"phase": "failed"},
	})

	// When
	err := classifyLocalReachability(context.Background(), deployment)

	// Then
	if !errors.Is(err, ErrLocalReachability) {
		t.Fatalf("expected v2 reachability classification, got %v", err)
	}
}

func TestRuntimeNetworkPathBlockedRequiresEveryForwardToBeBlocked(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		status   *runtimeadapter.RuntimeStatus
		expected bool
	}{
		{name: "missing status"},
		{name: "direct Podman has no VM", status: &runtimeadapter.RuntimeStatus{}},
		{
			name: "no forwards",
			status: &runtimeadapter.RuntimeStatus{
				VM: &runtimeadapter.VMDetails{},
			},
		},
		{
			name: "all blocked",
			status: &runtimeadapter.RuntimeStatus{
				VM: &runtimeadapter.VMDetails{Forwards: map[string]runtimeadapter.RuntimeEndpoint{
					"ssh":      {Health: "blocked"},
					"database": {Health: "timeout"},
				}},
			},
			expected: true,
		},
		{
			name: "refused means network path works",
			status: &runtimeadapter.RuntimeStatus{
				VM: &runtimeadapter.VMDetails{Forwards: map[string]runtimeadapter.RuntimeEndpoint{
					"ssh":      {Health: "blocked"},
					"database": {Health: "refused"},
				}},
			},
		},
		{
			name: "unknown health is not misclassified",
			status: &runtimeadapter.RuntimeStatus{
				VM: &runtimeadapter.VMDetails{Forwards: map[string]runtimeadapter.RuntimeEndpoint{
					"ssh": {Health: "starting"},
				}},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := runtimeNetworkPathBlocked(test.status); got != test.expected {
				t.Fatalf("runtimeNetworkPathBlocked() = %t, want %t", got, test.expected)
			}
		})
	}
}
