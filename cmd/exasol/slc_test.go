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
	"github.com/exasol/exasol-personal/internal/util"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func TestSLCCustomSubcommandsExist(t *testing.T) {
	t.Parallel()

	// When / Then
	for _, name := range []string{"install", "update"} {
		if !hasSubcommand(slcCustomCmd, name) {
			t.Fatalf("slc custom is missing the %q subcommand", name)
		}
	}
}

func TestSLCCustomRemoveIsHiddenInFavourOfUnifiedRemove(t *testing.T) {
	t.Parallel()

	// When / Then
	if !slcCustomRemoveCmd.Hidden {
		t.Fatal("slc custom remove must be hidden in favour of the unified slc remove")
	}
}

func TestSLCCustomOnlyFlagsAreNotOnOfficialInstall(t *testing.T) {
	t.Parallel()

	// When / Then
	for _, flag := range []string{"source", "alias", "language"} {
		if slcCustomInstallCmd.Flags().Lookup(flag) == nil {
			t.Fatalf("slc custom install is missing --%s", flag)
		}
		if slcInstallCmd.Flags().Lookup(flag) != nil {
			t.Fatalf("official install must not carry --%s", flag)
		}
	}
}

func TestSLCCustomInstallAndUpdateCarryRestartFlags(t *testing.T) {
	t.Parallel()

	// Given
	commands := map[string]*cobra.Command{
		"install": slcCustomInstallCmd,
		"update":  slcCustomUpdateCmd,
	}

	// When / Then
	for name, cmd := range commands {
		for _, flag := range []string{"no-restart", "auto-approve"} {
			if cmd.Flags().Lookup(flag) == nil {
				t.Fatalf("slc custom %s is missing --%s", name, flag)
			}
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

func TestSLCConfirmFunc_AutoApproveReturnsNilConfirmFunc(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "slc"}
	if slcConfirmFunc(cmd, true, "install") != nil {
		t.Fatal("expected --auto-approve to bypass confirmation entirely")
	}
}

func TestSLCConfirmFunc_NonInteractiveStdinRefusesWithGuidance(t *testing.T) {
	t.Parallel()

	if util.IsInteractiveStdin() {
		t.Skip("this process's stdin is a terminal; the non-interactive path isn't reachable here")
	}

	cmd := &cobra.Command{Use: "slc"}
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)

	fn := slcConfirmFunc(cmd, false, "install")
	if fn == nil {
		t.Fatal("expected a non-nil confirm func when --auto-approve is not set")
	}

	confirmed, err := fn()
	if confirmed {
		t.Fatal("expected non-interactive stdin to never auto-confirm")
	}
	if err == nil || !strings.Contains(err.Error(), "--auto-approve") {
		t.Fatalf("expected guidance to use --auto-approve, got %v", err)
	}
}

//nolint:paralleltest // mutates shared terminal message queues
func TestQueueSLCListJSON_QueuesFormattedJSON(t *testing.T) {
	resetTerminalMessages()
	defer resetTerminalMessages()

	statuses := []deploy.SLCStatus{{Flavor: "python-3.12"}}
	var customs []deploy.CustomSLCStatus
	if err := queueSLCListJSON(statuses, customs); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	var stdout, stderr bytes.Buffer
	writeTerminalMessages(terminalConfig{stdout: &stdout, stderr: &stderr, showCallsToAction: true})

	want, err := formatSLCListJSON(statuses, customs)
	if err != nil {
		t.Fatalf("failed to compute expected JSON: %v", err)
	}
	if strings.TrimSpace(stdout.String()) != want {
		t.Fatalf("expected queued JSON output %q, got %q", want, stdout.String())
	}
}

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
	writeTerminalMessages(terminalConfig{stdout: &stdout, stderr: &stderr, showCallsToAction: true})

	if strings.TrimSpace(stdout.String()) !=
		"No script language containers are available for this platform." {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
	if stderr.String() != "" {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}
}

func TestFormatSLCListTextRendersFlavorAliasVersionAndInstalledColumns(t *testing.T) {
	t.Parallel()

	output := formatSLCListText([]deploy.SLCStatus{
		{
			Flavor:    "python-3.12",
			Aliases:   []string{"PYTHON3", "PYTHON312"},
			Version:   "11.2.0",
			Installed: true,
		},
		{
			Flavor:    "java-17",
			Aliases:   []string{"JAVA", "JAVA17"},
			Version:   "11.2.0",
			Installed: false,
		},
	}, nil)

	if !strings.Contains(output, "FLAVOR") || !strings.Contains(output, "INSTALLED") {
		t.Fatalf("expected a header row, got %q", output)
	}
	if !strings.Contains(output, "python-3.12") || !strings.Contains(output, "PYTHON3, PYTHON312") {
		t.Fatalf("expected installed python row, got %q", output)
	}
	if !strings.Contains(output, "java-17") {
		t.Fatalf("expected java row, got %q", output)
	}

	lines := strings.Split(output, "\n")
	if len(lines) != 3 {
		t.Fatalf("expected a header plus one line per SLC, got %d lines: %q", len(lines), output)
	}
	if !strings.HasSuffix(strings.TrimRight(lines[1], " \t"), "yes") {
		t.Fatalf("expected installed python row to end in yes, got %q", lines[1])
	}
	if !strings.HasSuffix(strings.TrimRight(lines[2], " \t"), "no") {
		t.Fatalf("expected non-installed java row to end in no, got %q", lines[2])
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
	writeTerminalMessages(terminalConfig{stdout: &stdout, stderr: &stderr, showCallsToAction: true})

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

//nolint:paralleltest // mutates shared terminal message queues
func TestPrintSLCInstallResultDescribesOutcomeAndReplacement(t *testing.T) {
	cases := map[string]struct {
		result   *deploy.SLCInstallResult
		wantText []string
	}{
		"fresh install, deferred activation": {
			result: &deploy.SLCInstallResult{
				Entry:   config.InstalledSLC{Flavor: "python-3.12", Aliases: []string{"PYTHON3"}},
				Outcome: deploy.SLCApplyDeferred,
			},
			wantText: []string{"Installed python-3.12", "next start"},
		},
		"replacing an existing flavor, database restarted": {
			result: &deploy.SLCInstallResult{
				Entry:    config.InstalledSLC{Flavor: "python-3.12", Aliases: []string{"PYTHON3"}},
				Replaced: true,
				Outcome:  deploy.SLCApplyRestarted,
			},
			wantText: []string{"Updated python-3.12", "restarted"},
		},
		"database started to apply the change": {
			result: &deploy.SLCInstallResult{
				Entry:   config.InstalledSLC{Flavor: "python-3.12", Aliases: []string{"PYTHON3"}},
				Outcome: deploy.SLCApplyStarted,
			},
			wantText: []string{"Installed python-3.12", "started"},
		},
	}

	for name, testCase := range cases {
		//nolint:paralleltest // mutates shared terminal message queues
		t.Run(name, func(t *testing.T) {
			resetTerminalMessages()
			defer resetTerminalMessages()

			printSLCInstallResult(testCase.result)

			stdout := bytes.Buffer{}
			writeTerminalMessages(terminalConfig{stdout: &stdout, showCallsToAction: true})
			for _, want := range testCase.wantText {
				if !strings.Contains(stdout.String(), want) {
					t.Fatalf("expected output to contain %q, got %q", want, stdout.String())
				}
			}
		})
	}
}

//nolint:paralleltest // mutates shared terminal message queues
func TestPrintSLCUpdateResultDescribesFlavorChangeAndOutcome(t *testing.T) {
	resetTerminalMessages()
	defer resetTerminalMessages()

	printSLCUpdateResult(&deploy.SLCUpdateResult{
		Entry:       &config.InstalledSLC{Flavor: "python-3.13"},
		FromFlavor:  "python-3.12",
		FromVersion: "11.1.0",
		Outcome:     deploy.SLCApplyRestarted,
	})

	stdout := bytes.Buffer{}
	writeTerminalMessages(terminalConfig{stdout: &stdout, showCallsToAction: true})

	if !strings.Contains(stdout.String(), "Updated python-3.12 to python-3.13") {
		t.Fatalf("expected flavor transition in output, got %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "restarted") {
		t.Fatalf("expected restart outcome in output, got %q", stdout.String())
	}
}

//nolint:paralleltest // mutates shared terminal message queues
func TestPrintSLCUpdateResultDescribesStoppedDatabaseStarted(t *testing.T) {
	resetTerminalMessages()
	defer resetTerminalMessages()

	printSLCUpdateResult(&deploy.SLCUpdateResult{
		Entry:   &config.InstalledSLC{Flavor: "python-3.12"},
		Outcome: deploy.SLCApplyStarted,
	})

	stdout := bytes.Buffer{}
	writeTerminalMessages(terminalConfig{stdout: &stdout, showCallsToAction: true})

	if !strings.Contains(stdout.String(), "Updated python-3.12") {
		t.Fatalf("expected update message, got %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "started") {
		t.Fatalf("expected started outcome in output, got %q", stdout.String())
	}
}

//nolint:paralleltest // mutates shared terminal message queues
func TestPrintSLCRemoveResultDescribesOutcome(t *testing.T) {
	resetTerminalMessages()
	defer resetTerminalMessages()

	printSLCRemoveResult(&deploy.SLCRemoveResult{
		Entry:   &config.InstalledSLC{Flavor: "python-3.12"},
		Outcome: deploy.SLCApplyDeferred,
	})

	stdout := bytes.Buffer{}
	writeTerminalMessages(terminalConfig{stdout: &stdout, showCallsToAction: true})

	if !strings.Contains(stdout.String(), "Removed python-3.12") {
		t.Fatalf("expected removal message, got %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "next start") {
		t.Fatalf("expected deferred-activation wording, got %q", stdout.String())
	}
}

//nolint:paralleltest // mutates shared terminal message queues
func TestPrintSLCRemoveResultDescribesDatabaseRestarted(t *testing.T) {
	resetTerminalMessages()
	defer resetTerminalMessages()

	printSLCRemoveResult(&deploy.SLCRemoveResult{
		Entry:   &config.InstalledSLC{Flavor: "python-3.12"},
		Outcome: deploy.SLCApplyRestarted,
	})

	stdout := bytes.Buffer{}
	writeTerminalMessages(terminalConfig{stdout: &stdout, showCallsToAction: true})

	if !strings.Contains(stdout.String(), "Removed python-3.12") {
		t.Fatalf("expected removal message, got %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "restarted") {
		t.Fatalf("expected restarted wording, got %q", stdout.String())
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
	// Given
	for _, cmd := range []struct {
		name string
		cmd  interface{ Flag(name string) *pflag.Flag }
	}{
		{name: "install", cmd: slcInstallCmd},
		{name: "update", cmd: slcUpdateCmd},
		{name: "remove", cmd: slcRemoveCmd},
		{name: "custom install", cmd: slcCustomInstallCmd},
		{name: "custom update", cmd: slcCustomUpdateCmd},
		{name: "custom remove", cmd: slcCustomRemoveCmd},
	} {
		// When / Then
		if cmd.cmd.Flag("json") == nil {
			t.Fatalf("expected slc %s to register --json", cmd.name)
		}
	}
}

//nolint:paralleltest // mutates shared terminal message queues
func TestRenderCustomSLCCommandJSONCarriesTheContract(t *testing.T) {
	resetTerminalMessages()
	defer resetTerminalMessages()

	// Given
	result := &deploy.CustomSLCInstallResult{
		Operation: deploy.SLCOperationInstall,
		Alias:     "MYPY3",
		Language:  "python",
		Changed:   true,
		Outcome:   deploy.SLCApplyRestarted,
	}

	// When
	if err := renderSLCCommandJSON(result); err != nil {
		t.Fatalf("failed to render JSON: %v", err)
	}

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	writeTerminalMessages(terminalConfig{stdout: &stdout, stderr: &stderr, showCallsToAction: true})

	// Then
	if stderr.String() != "" {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}

	var decoded map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("stdout is not parseable JSON: %v\n%s", err, stdout.String())
	}
	for key, want := range map[string]any{
		"operation": "install",
		"alias":     "MYPY3",
		"outcome":   "restarted",
	} {
		if decoded[key] != want {
			t.Fatalf("%s = %v, want %v", key, decoded[key], want)
		}
	}
}

// A prompt on stdout would corrupt --json output, so both confirmation paths must use stderr.
func TestSLCConfirmPromptsGoToStderr(t *testing.T) {
	t.Parallel()

	for name, prompt := range map[string]func(*cobra.Command) error{
		"custom": func(cmd *cobra.Command) error {
			_, err := customSLCConfirmFunc(cmd, false)("overriding a built-in alias")

			return err
		},
		"official": func(cmd *cobra.Command) error {
			_, err := slcConfirmFunc(cmd, false, "Installing")()

			return err
		},
	} {
		// Given: a command whose streams are captured, and a declining answer on stdin.
		stdout := bytes.Buffer{}
		stderr := bytes.Buffer{}
		cmd := &cobra.Command{}
		cmd.SetOut(&stdout)
		cmd.SetErr(&stderr)
		cmd.SetIn(strings.NewReader("n\n"))

		// When
		_ = prompt(cmd)

		// Then
		if stdout.Len() != 0 {
			t.Fatalf("%s: prompt must not write to stdout, got %q", name, stdout.String())
		}
	}
}

// The blank line is the one thing the per-table split could have changed.
func TestFormatSLCListTextSeparatesOfficialFromCustom(t *testing.T) {
	t.Parallel()

	// Given
	official := []deploy.SLCStatus{
		{Language: "python", Flavor: "python-3.12", Version: "3.12", Aliases: []string{"PYTHON3"}},
	}
	custom := []deploy.CustomSLCStatus{{Alias: "MYPY3", Language: "python", Source: "my.tar.gz"}}

	// When
	output := formatSLCListText(official, custom)

	// Then
	if !strings.Contains(output, "FLAVOR") || !strings.Contains(output, "CUSTOM ALIAS") {
		t.Fatalf("expected both tables, got %q", output)
	}
	if !strings.Contains(output, "\n\n") {
		t.Fatalf("expected a blank line between the two tables, got %q", output)
	}

	// And: a custom-only listing must not start with a blank line.
	if got := formatSLCListText(nil, custom); strings.HasPrefix(got, "\n") {
		t.Fatalf("a custom-only listing must not be preceded by a blank line, got %q", got)
	}
}
