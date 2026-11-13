// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package presets

import "testing"

func TestParseInstallManifest_ReadsCompatibilityRequirements(t *testing.T) {
	t.Parallel()

	// Given
	manifestRaw := []byte(
		"name: Test Install\n" +
			"description: test install\n" +
			"compatibility:\n" +
			"  requires:\n" +
			"    - remote-exec\n" +
			"install:\n" +
			"  - remoteExec:\n" +
			"      description: run remotely\n" +
			"      filename: monitor.sh\n",
	)

	// When
	manifest, err := parseInstallManifest(manifestRaw)
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got := manifest.RequiredCapabilities(); len(got) != 1 || got[0] != "remote-exec" {
		t.Fatalf("unexpected compatibility requirements: %#v", got)
	}
	if len(manifest.Install) != 1 {
		t.Fatalf("expected 1 install step, got %d", len(manifest.Install))
	}
	if manifest.Install[0].RemoteExec == nil {
		t.Fatal("expected remoteExec step to be populated")
	}
}

func TestParseInstallManifest_ReadsLocalCommandSteps(t *testing.T) {
	t.Parallel()

	// Given
	manifestRaw := []byte(
		"name: Test Install\n" +
			"description: test install\n" +
			"compatibility:\n" +
			"  requires:\n" +
			"    - local-command\n" +
			"install:\n" +
			"  - localCommand:\n" +
			"      description: run locally\n" +
			"      command: [\"echo\", \"hello\"]\n",
	)

	// When
	manifest, err := parseInstallManifest(manifestRaw)
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got := manifest.RequiredCapabilities(); len(got) != 1 || got[0] != "local-command" {
		t.Fatalf("unexpected compatibility requirements: %#v", got)
	}
	if len(manifest.Install) != 1 {
		t.Fatalf("expected 1 install step, got %d", len(manifest.Install))
	}
	if manifest.Install[0].LocalCommand == nil {
		t.Fatal("expected localCommand step to be populated")
	}
}

func TestParseInstallManifest_DatabaseConnectionSupport(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name     string
		metadata string
		want     bool
	}{
		{name: "legacy"},
		{name: "enabled", metadata: "databaseConnection:\n  allowAllNodes: true\n", want: true},
		{name: "disabled", metadata: "databaseConnection:\n  allowAllNodes: false\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Given
			raw := []byte("name: Test\n" + test.metadata)
			// When
			manifest, err := parseInstallManifest(raw)
			// Then
			if err != nil {
				t.Fatal(err)
			}
			if manifest.DatabaseConnection.AllowAllNodes != test.want {
				t.Fatalf("expected multi-node support %v", test.want)
			}
		})
	}
}
