// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"strings"
	"testing"
)

func TestRootCmdLongDescListsLocalPresetFirst(t *testing.T) {
	t.Parallel()

	if !strings.Contains(
		rootCmdLongDesc,
		"Built-in presets are: local, aws, azure, exoscale, and stackit.",
	) {
		t.Fatalf(
			"expected local to be listed first among built-in presets, got: %s",
			rootCmdLongDesc,
		)
	}
}

func TestRootCmdLongDescDocumentsLocalQuickStartAndLifecycle(t *testing.T) {
	t.Parallel()

	if !strings.Contains(rootCmdLongDesc, "exasol install local") {
		t.Fatalf("expected a local quick-start pointer, got: %s", rootCmdLongDesc)
	}

	lifecycleCommands := []string{"exasol status", "exasol connect", "exasol stop", "exasol start"}
	for _, lifecycleCmd := range lifecycleCommands {
		if !strings.Contains(rootCmdLongDesc, lifecycleCmd) {
			t.Fatalf(
				"expected local lifecycle command %q to be documented, got: %s",
				lifecycleCmd,
				rootCmdLongDesc,
			)
		}
	}
}

func TestRootCmdExampleUsesLocalPreset(t *testing.T) {
	t.Parallel()

	if rootCmdExample != "  exasol install local" {
		t.Fatalf("expected root example to use the local preset, got: %q", rootCmdExample)
	}
}
