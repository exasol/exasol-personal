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
	"github.com/spf13/pflag"
)

func TestRenderSLCListTextNoneAvailable(t *testing.T) {
	t.Parallel()

	output := formatSLCListText([]deploy.SLCStatus{})

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

	renderSLCListText([]deploy.SLCStatus{})

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	writeTerminalMessages(&stdout, &stderr, true)

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

	var buf bytes.Buffer
	if err := renderSLCListJSON(&buf, []deploy.SLCStatus{}); err != nil {
		t.Fatalf("expected json render to succeed, got %v", err)
	}

	if got := strings.TrimSpace(buf.String()); got != "[]" {
		t.Fatalf("expected empty JSON array, got %q", got)
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
	writeTerminalMessages(&stdout, &stderr, true)

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
