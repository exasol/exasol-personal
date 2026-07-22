// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func TestSLCCommandSeparation(t *testing.T) {
	t.Parallel()

	for name, cmd := range map[string]*cobra.Command{
		"install": slcCustomInstallCmd,
		"update":  slcCustomUpdateCmd,
		"remove":  slcCustomRemoveCmd,
	} {
		if !hasSubcommand(slcCustomCmd, name) {
			t.Fatalf("slc custom is missing the %q subcommand", name)
		}
		if cmd.Flags().Lookup("no-restart") != nil {
			t.Fatalf("custom %s must not carry --no-restart", name)
		}
	}

	for _, flag := range []string{"file", "url", "alias", "language"} {
		if slcCustomInstallCmd.Flags().Lookup(flag) == nil {
			t.Fatalf("slc custom install is missing --%s", flag)
		}
		if slcInstallCmd.Flags().Lookup(flag) != nil {
			t.Fatalf("official install must not carry --%s", flag)
		}
	}
	if slcInstallCmd.Flags().Lookup("no-restart") == nil {
		t.Fatal("official install must keep --no-restart")
	}
}

func hasSubcommand(parent *cobra.Command, name string) bool {
	for _, sub := range parent.Commands() {
		if sub.Name() == name {
			return true
		}
	}

	return false
}

// On an unsupported architecture `slc list` degrades gracefully: SLCStatuses returns an
// empty set (no error), which the renderers must present as the "none available" message
// and an empty JSON array — never an error or a non-zero exit.

func TestRenderSLCListTextNoneAvailable(t *testing.T) {
	t.Parallel()

	// Given
	official := []deploy.SLCStatus{}
	custom := []deploy.CustomSLCStatus{}

	// When
	output := formatSLCListText(official, custom)

	// Then
	if strings.TrimSpace(
		output,
	) != "No script language containers are available for this platform." {
		t.Fatalf("unexpected text output: %q", output)
	}
}

//nolint:paralleltest // mutates shared terminal message queues
func TestRenderSLCListTextQueuesPrimaryOutput(t *testing.T) {
	resetTerminalMessages()
	defer resetTerminalMessages()

	renderSLCListText([]deploy.SLCStatus{}, []deploy.CustomSLCStatus{})

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	writeTerminalMessages(&stdout, &stderr)

	if strings.TrimSpace(stdout.String()) !=
		"No script language containers are available for this platform." {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
	if stderr.String() != "" {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}
}

func TestRenderSLCListJSONEmptyIsArray(t *testing.T) {
	t.Parallel()

	// Given
	var buf bytes.Buffer

	// When
	err := renderSLCListJSON(&buf, []deploy.SLCStatus{}, []deploy.CustomSLCStatus{})
	// Then
	if err != nil {
		t.Fatalf("expected json render to succeed, got %v", err)
	}
	if got := strings.TrimSpace(buf.String()); got != "[]" {
		t.Fatalf("expected empty JSON array, got %q", got)
	}
}

// A custom SLC appears in both renderers, distinguished by type in JSON and a separate
// section in text, so `slc list` covers custom containers alongside official ones.

func TestRenderSLCListTextIncludesCustom(t *testing.T) {
	t.Parallel()

	// Given
	custom := []deploy.CustomSLCStatus{{Alias: "MYPY3", Language: "python", Source: "my.tar.gz"}}

	// When
	output := formatSLCListText([]deploy.SLCStatus{}, custom)

	// Then
	if !strings.Contains(output, "CUSTOM ALIAS") || !strings.Contains(output, "MYPY3") {
		t.Fatalf("expected custom section with the alias, got %q", output)
	}
}

func TestRenderSLCListJSONIncludesCustomType(t *testing.T) {
	t.Parallel()

	// Given
	var buf bytes.Buffer
	custom := []deploy.CustomSLCStatus{{Alias: "MYPY3", Language: "python", Source: "my.tar.gz"}}

	// When
	err := renderSLCListJSON(&buf, []deploy.SLCStatus{}, custom)
	// Then
	if err != nil {
		t.Fatalf("expected json render to succeed, got %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, `"type": "custom"`) || !strings.Contains(got, `"alias": "MYPY3"`) {
		t.Fatalf("expected custom-typed JSON item, got %q", got)
	}
}

//nolint:paralleltest // mutates shared terminal message queues
func TestRenderSLCCommandJSONQueuesParseablePrimaryOutput(t *testing.T) {
	resetTerminalMessages()
	defer resetTerminalMessages()

	result := &deploy.SLCInstallResult{
		Operation: deploy.SLCOperationInstall,
		Entry: config.InstalledSLC{
			Language: "python",
			Flavor:   "python-3.12",
			Version:  "3.12",
			Image:    "docker.io/exasol/script-language-container:python-3.12",
			Target:   "/exa/slc/python-3.12",
			Aliases:  []string{"PYTHON3", "PYTHON312"},
		},
		Changed: true,
		Outcome: deploy.SLCApplyDeferred,
	}

	if err := renderSLCCommandJSON(result); err != nil {
		t.Fatalf("failed to render JSON: %v", err)
	}

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	writeTerminalMessages(&stdout, &stderr)

	if stderr.String() != "" {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}

	var decoded map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("stdout is not parseable JSON: %v\n%s", err, stdout.String())
	}
	if decoded["operation"] != "install" {
		t.Fatalf("unexpected operation: %v", decoded["operation"])
	}
	if decoded["outcome"] != "deferred" {
		t.Fatalf("unexpected outcome: %v", decoded["outcome"])
	}
}

func TestSLCApplyOutcomeStringReportsNoOp(t *testing.T) {
	t.Parallel()

	if got := deploy.SLCApplyNone.String(); got != "none" {
		t.Fatalf("expected no-op outcome, got %q", got)
	}
}

//nolint:paralleltest // reads package-global Cobra commands
func TestSLCMutationCommandsRegisterJSONFlag(t *testing.T) {
	for _, cmd := range []struct {
		name string
		cmd  interface{ Flag(name string) *pflag.Flag }
	}{
		{name: "install", cmd: slcInstallCmd},
		{name: "update", cmd: slcUpdateCmd},
		{name: "remove", cmd: slcRemoveCmd},
	} {
		if cmd.cmd.Flag("json") == nil {
			t.Fatalf("expected slc %s to register --json", cmd.name)
		}
	}
}
