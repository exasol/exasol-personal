// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestNoLauncherVersionCheckFlagAppearsInTheCommandHelp(t *testing.T) {
	t.Parallel()

	// Given a command that registers the init flags and renders the launcher's usage
	cmd := &cobra.Command{
		Use:  "init",
		RunE: func(*cobra.Command, []string) error { return nil },
	}
	cmd.SetUsageTemplate(customUsageTemplate)
	registerInitFlags(cmd, &CommonFlags{})

	// When the help is rendered
	usage := cmd.UsageString()

	// Then the flag is listed, so it reaches the documented reference
	if !strings.Contains(usage, "--no-launcher-version-check") {
		t.Errorf("expected --no-launcher-version-check in the help output:\n%s", usage)
	}
}

func TestNoLauncherVersionCheckFlagSetsTheFlagState(t *testing.T) {
	t.Parallel()

	// Given a command that registers the init flags
	state := &CommonFlags{}
	cmd := &cobra.Command{
		Use:  "init",
		RunE: func(*cobra.Command, []string) error { return nil },
	}
	registerInitFlags(cmd, state)

	// When the flag is passed
	cmd.SetArgs([]string{"--no-launcher-version-check"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected the command to accept --no-launcher-version-check: %v", err)
	}

	// Then the state records it
	if !state.NoLauncherVersionCheck {
		t.Error("expected --no-launcher-version-check to set the flag state")
	}
}
