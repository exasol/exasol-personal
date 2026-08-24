// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReadRunnerStateUsesLabeledForwardsWithoutTransportMetadata(t *testing.T) {
	t.Parallel()

	statePath := filepath.Join(t.TempDir(), "vm-state.json")
	state := []byte(`{
  "vm_name": "exasol-local-vm",
  "shared_dir": "./vm-shared",
  "forwards": {"db": {"guest_port": 8563, "host_port": 28563}}
}`)
	if err := os.WriteFile(statePath, state, 0o600); err != nil {
		t.Fatalf("failed to write state: %v", err)
	}

	parsed, err := readRunnerState(statePath)
	if err != nil {
		t.Fatalf("failed to parse state: %v", err)
	}
	endpoint, err := runtimeEndpointFromRunnerState(parsed)
	if err != nil || endpoint.DBPort != 28563 {
		t.Fatalf("unexpected endpoint %#v, err=%v", endpoint, err)
	}
}

func TestReadRunnerStateRejectsMissingOrWrongDatabaseForward(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"missing":      `{"forwards":{}}`,
		"wrong guest":  `{"forwards":{"db":{"guest_port":9999,"host_port":28563}}}`,
		"invalid host": `{"forwards":{"db":{"guest_port":8563,"host_port":0}}}`,
	}
	for name, state := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			statePath := filepath.Join(t.TempDir(), "vm-state.json")
			if err := os.WriteFile(statePath, []byte(state), 0o600); err != nil {
				t.Fatalf("failed to write state: %v", err)
			}
			if _, err := readRunnerState(statePath); err == nil {
				t.Fatal("expected invalid state to fail")
			}
		})
	}
}

func TestResolveMacHostDBPort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ports string
		want  int
	}{
		{ports: "", want: 8563},
		{ports: "auto", want: 0},
		{ports: "db:0", want: 0},
		{ports: "db:28563", want: 28563},
		{ports: "ssh:20022, db:28563", want: 28563},
	}
	for _, test := range tests {
		actual, err := resolveMacHostDBPort(test.ports)
		if err != nil || actual != test.want {
			t.Fatalf(
				"resolveMacHostDBPort(%q) = %d, %v; want %d",
				test.ports,
				actual,
				err,
				test.want,
			)
		}
	}

	for _, ports := range []string{"db", "db:-1", "db:65536", "db:1,db:2"} {
		if _, err := resolveMacHostDBPort(ports); err == nil {
			t.Fatalf("expected invalid mapping %q to fail", ports)
		}
	}
}

func TestMaterializeFileAtomicallyStagesAndReusesUnchangedArtifact(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sourcePath := filepath.Join(root, "source.tar")
	targetPath := filepath.Join(root, "share", "nano.tar")
	if err := os.WriteFile(sourcePath, []byte("image"), 0o600); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}
	modTime := time.Unix(1_700_000_000, 123)
	if err := os.Chtimes(sourcePath, modTime, modTime); err != nil {
		t.Fatalf("failed to set source time: %v", err)
	}
	if err := materializeFileAtomically(sourcePath, targetPath); err != nil {
		t.Fatalf("failed to stage artifact: %v", err)
	}
	firstInfo, err := os.Stat(targetPath)
	if err != nil {
		t.Fatalf("failed to stat target: %v", err)
	}
	if firstInfo.Mode().Perm() != 0o640 || !firstInfo.ModTime().Equal(modTime) {
		t.Fatalf(
			"unexpected staged metadata: mode=%o time=%s",
			firstInfo.Mode().Perm(),
			firstInfo.ModTime(),
		)
	}
	if err := materializeFileAtomically(sourcePath, targetPath); err != nil {
		t.Fatalf("failed to reuse staged artifact: %v", err)
	}
	secondInfo, err := os.Stat(targetPath)
	if err != nil {
		t.Fatalf("failed to stat reused target: %v", err)
	}
	if !os.SameFile(firstInfo, secondInfo) {
		t.Fatal("expected unchanged staged artifact not to be replaced")
	}
}

func TestMaterializeFileAtomicallyRejectsDirectorySource(t *testing.T) {
	t.Parallel()

	err := materializeFileAtomically(t.TempDir(), filepath.Join(t.TempDir(), "target"))
	if err == nil || !strings.Contains(err.Error(), "directory") {
		t.Fatalf("expected directory source error, got %v", err)
	}
}

func TestValidatePort(t *testing.T) {
	t.Parallel()

	if err := validatePort("database", 8563); err != nil {
		t.Fatalf("expected valid port: %v", err)
	}
	if err := validatePort("database", 0); err == nil {
		t.Fatalf("expected invalid port error, got %v", err)
	}
}
