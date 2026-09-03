// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/deploy"
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

// `slc install rust` and `slc update rust` are dispatched by alias, so they must not have grown
// custom-SLC flags: the shared command keeps one uniform contract for every alias.
func TestSLCCustomOnlyFlagsAreNotOnOfficialCommands(t *testing.T) {
	t.Parallel()

	// Given
	official := map[string]*cobra.Command{"install": slcInstallCmd, "update": slcUpdateCmd}

	// When / Then
	for _, flag := range []string{"source", "alias", "language"} {
		if slcCustomInstallCmd.Flags().Lookup(flag) == nil {
			t.Fatalf("slc custom install is missing --%s", flag)
		}
		for name, cmd := range official {
			if cmd.Flags().Lookup(flag) != nil {
				t.Fatalf("official %s must not carry --%s", name, flag)
			}
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

// Rust support must not widen the flag surface of the shared install/update commands, so the
// expected flag set is asserted exactly rather than only checking for known additions.
func TestSLCInstallAndUpdateFlagSurfaceIsUnchangedByRustSupport(t *testing.T) {
	t.Parallel()

	// Given
	want := []string{
		"auto-approve", "no-restart", "deployment", "deployment-dir", "json", "verbose",
	}

	// When / Then
	for name, cmd := range map[string]*cobra.Command{
		"install": slcInstallCmd,
		"update":  slcUpdateCmd,
	} {
		for _, flag := range want {
			if cmd.Flags().Lookup(flag) == nil {
				t.Errorf("slc %s is missing --%s", name, flag)
			}
		}
		cmd.Flags().VisitAll(func(flag *pflag.Flag) {
			if !slices.Contains(want, flag.Name) {
				t.Errorf("slc %s carries unexpected flag --%s", name, flag.Name)
			}
		})
	}
}

// The `rust` alias is a documented part of the shared commands, so dropping it from the help
// would silently hide the only way to discover it.
func TestSLCInstallAndUpdateDocumentTheRustAlias(t *testing.T) {
	t.Parallel()

	// When / Then
	for name, long := range map[string]string{
		"install": slcInstallCmd.Long,
		"update":  slcUpdateCmd.Long,
	} {
		for _, want := range []string{
			"`rust`",
			"exasol-labs/language-container-rs",
			"exasol slc custom install --language rust --source",
		} {
			if !strings.Contains(long, want) {
				t.Errorf("slc %s help does not mention %q", name, want)
			}
		}
	}
}

// The alias dispatch is the only thing wiring the Rust SLC into the shared commands; keep both
// entry points at the signature the RunE handlers call them with.
func TestSLCRustDispatchFunctionsAreWired(t *testing.T) {
	t.Parallel()

	// Given: the dispatch targets, typed as the RunE handlers call them.
	dispatch := map[string]func(*cobra.Command) error{
		"install": runSLCInstallRust,
		"update":  runSLCUpdateRust,
	}

	// When / Then
	for name, run := range dispatch {
		if run == nil {
			t.Errorf("slc %s has no Rust dispatch target", name)
		}
	}

	if !strings.EqualFold("rust", deploy.RustSLCAlias) {
		t.Fatalf("the documented alias `rust` does not match deploy.RustSLCAlias (%q)",
			deploy.RustSLCAlias)
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
