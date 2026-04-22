// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package presets

import "testing"

func TestParseInstallManifest_ReadsLocalCommandSteps(t *testing.T) {
	t.Parallel()

	// Given
	manifestRaw := []byte(`
name: Test Install
description: test install
compatibility:
  requires:
    - local-command
install:
  - localCommand:
      description: run locally
      command: ["echo", "hello"]
`)

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
