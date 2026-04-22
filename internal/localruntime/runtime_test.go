// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"fmt"
	"os"
	"testing"
)

func TestRuntimeEnsureRoot_CreatesDeploymentScopedLayout(t *testing.T) {
	t.Parallel()

	// Given
	runtime := New(t.TempDir())

	// When
	err := runtime.EnsureRoot()

	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	for _, path := range []string{
		runtime.Layout().RuntimeRoot(),
		runtime.Layout().ConfigDir(),
		runtime.Layout().BootstrapDir(),
		runtime.Layout().ControlDir(),
		runtime.Layout().DataDir(),
		runtime.Layout().LogsDir(),
		runtime.Layout().VMDir(),
		runtime.Layout().PayloadDir(),
	} {
		info, statErr := os.Stat(path)
		if statErr != nil {
			t.Fatalf("expected %s to exist, got %v", path, statErr)
		}
		if !info.IsDir() {
			t.Fatalf("expected %s to be a directory", path)
		}
	}
}

func TestRuntimeAllocatePort_PersistsAndReusesAssignments(t *testing.T) {
	t.Parallel()

	// Given
	runtime := New(t.TempDir())
	nextPort := 10000
	runtime.allocatePort = func(excluded map[int]struct{}) (int, error) {
		for ; nextPort < 10100; nextPort++ {
			if _, exists := excluded[nextPort]; exists {
				continue
			}
			port := nextPort
			nextPort++
			return port, nil
		}

		return 0, fmt.Errorf("no ports left")
	}

	// When
	dbPort, err := runtime.AllocatePort("db")
	if err != nil {
		t.Fatalf("expected db port allocation to succeed, got %v", err)
	}
	dbPortAgain, err := runtime.AllocatePort("db")
	if err != nil {
		t.Fatalf("expected db port reuse to succeed, got %v", err)
	}
	uiPort, err := runtime.AllocatePort("ui")
	if err != nil {
		t.Fatalf("expected ui port allocation to succeed, got %v", err)
	}

	// Then
	if dbPort <= 0 {
		t.Fatalf("expected positive db port, got %d", dbPort)
	}
	if dbPort != dbPortAgain {
		t.Fatalf("expected db port to be reused, got %d then %d", dbPort, dbPortAgain)
	}
	if uiPort <= 0 {
		t.Fatalf("expected positive ui port, got %d", uiPort)
	}
	if uiPort == dbPort {
		t.Fatalf("expected different ports for ui and db, both were %d", uiPort)
	}
}
