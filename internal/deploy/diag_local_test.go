// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDiagnoseLocalUsesV2ProviderAndDoesNotMutateState(t *testing.T) {
	// Given
	t.Setenv(localAllowUnsupportedEnv, "1")
	deployment := newLocalTestDeployment(t)
	installLocalRuntimeTestAssets(t)
	writeFakeV2Provider(t, deployment, map[string]any{
		"schemaVersion": 1,
		"phase":         "stopped",
		"forwards": []map[string]any{{
			"name": "database", "hostAddress": "127.0.0.1", "hostPort": 8563,
		}},
		"hook": map[string]any{"phase": "not-run"},
	})
	before, err := os.ReadFile(deployment.ExasolPersonalStatePath())
	if err != nil {
		t.Fatal(err)
	}

	// When
	diagnostics := diagnoseLocalUnsafe(context.Background(), deployment)

	// Then
	if diagnostics.VMRunning == nil || *diagnostics.VMRunning {
		t.Fatalf("expected stopped v2 provider diagnostics, got %#v", diagnostics)
	}
	if diagnostics.RuntimeRunning == nil || *diagnostics.RuntimeRunning ||
		diagnostics.RuntimeKind != "local-vm" {
		t.Fatalf("runtime ownership is missing from diagnostics: %#v", diagnostics)
	}
	after, err := os.ReadFile(deployment.ExasolPersonalStatePath())
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("diagnostics changed persistent launcher state")
	}
}

func writeFakeV2Provider(
	t *testing.T,
	deployment interface{ Root() string },
	state map[string]any,
) {
	t.Helper()
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(deployment.Root(), "local", "provider", "local-vm")
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\n" +
		"case \"$1\" in\n" +
		"  status|health-check) echo '" + string(data) + "' ;;\n" +
		"  version) echo '{\"version\":\"dev\"," +
		"\"configSchemaVersion\":1,\"hookAPIVersion\":1," +
		"\"stateSchemaVersion\":1}' ;;\n" +
		"  stop|destroy) exit 0 ;;\n" +
		"  *) exit 1 ;;\n" +
		"esac\n"
	//nolint:gosec // The fake provider fixture must be executable.
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
}
