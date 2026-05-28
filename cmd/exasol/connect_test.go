// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/connect"
	"github.com/spf13/pflag"
)

func TestJSONFormatValueSet(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name     string
		input    string
		expected connect.JSONFormat
	}{
		{name: "empty defaults to pretty", input: "", expected: connect.JSONFormatPretty},
		{
			name:     "trims and lowercases compact",
			input:    "  COMPACT  ",
			expected: connect.JSONFormatCompact,
		},
		{name: "keeps pretty", input: "pretty", expected: connect.JSONFormatPretty},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var target connect.JSONFormat
			value := NewJSONFormatValue(&target, connect.JSONFormatPretty)

			if err := value.Set(test.input); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if target != test.expected {
				t.Fatalf("unexpected parsed format: got %q expected %q", target, test.expected)
			}
		})
	}
}

func TestJSONFormatValueSetRejectsInvalidValue(t *testing.T) {
	t.Parallel()

	var target connect.JSONFormat
	value := NewJSONFormatValue(&target, connect.JSONFormatPretty)

	err := value.Set("yaml")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "expected one of") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestJSONFormatVarP_SetsPrettyWhenFlagHasNoValue(t *testing.T) {
	t.Parallel()

	flagSet := pflag.NewFlagSet("connect", pflag.ContinueOnError)
	var target connect.JSONFormat
	JSONFormatVarP(
		flagSet,
		&target,
		"json",
		"j",
		connect.JSONFormatPretty,
		"Output in JSON format: pretty, compact",
	)

	if err := flagSet.Parse([]string{"--json"}); err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if target != connect.JSONFormatPretty {
		t.Fatalf("unexpected format: got %q expected %q", target, connect.JSONFormatPretty)
	}
	if !flagSet.Changed("json") {
		t.Fatal("expected json flag to be marked changed")
	}
}

func TestJSONFormatVarP_AcceptsExplicitCompactValue(t *testing.T) {
	t.Parallel()

	flagSet := pflag.NewFlagSet("connect", pflag.ContinueOnError)
	var target connect.JSONFormat
	JSONFormatVarP(
		flagSet,
		&target,
		"json",
		"j",
		connect.JSONFormatPretty,
		"Output in JSON format: pretty, compact",
	)

	if err := flagSet.Parse([]string{"--json=compact"}); err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if target != connect.JSONFormatCompact {
		t.Fatalf("unexpected format: got %q expected %q", target, connect.JSONFormatCompact)
	}
}

func TestConnectCmdExamplesMentionJSONOptions(t *testing.T) {
	t.Parallel()

	for _, expected := range []string{
		"--json",
		"printf 'SELECT 1;\\n' | exasol connect --json=compact",
	} {
		if !strings.Contains(connectCmdExample, expected) {
			t.Fatalf("expected examples to contain %q", expected)
		}
	}
}

// nolint: paralleltest // Mutates the shared command usage template for inspection.
func TestConnectUsageShowsJSONFormatUnderFlags(t *testing.T) {
	originalTemplate := connectCmd.UsageTemplate()
	connectCmd.SetUsageTemplate(customUsageTemplate)
	t.Cleanup(func() {
		connectCmd.SetUsageTemplate(originalTemplate)
	})

	// Match the runtime setup where help is added explicitly.
	if connectCmd.Flags().Lookup("help") == nil {
		connectCmd.Flags().BoolP("help", "h", false, "Help for connect")
	}

	usage := connectCmd.UsageString()
	if !strings.Contains(usage, "--json string") {
		t.Fatalf("expected usage to list --json under flags, got:\n%s", usage)
	}
	if !strings.Contains(usage, "Output in JSON format: pretty, compact") {
		t.Fatalf("expected usage to describe --json, got:\n%s", usage)
	}
	if !strings.Contains(usage, "Examples:") {
		t.Fatalf("expected usage to include examples, got:\n%s", usage)
	}
	if !strings.Contains(usage, connectCmdExample) {
		t.Fatalf("expected usage to include connect examples, got:\n%s", usage)
	}

	// Sanity check: the flag is local to the connect command, not persistent/inherited.
	jsonFlag := connectCmd.LocalNonPersistentFlags().Lookup("json")
	if jsonFlag == nil {
		t.Fatal("expected --json to be a local non-persistent flag")

		return
	}
	if jsonFlag.NoOptDefVal != connect.JSONFormatPretty.String() {
		t.Fatalf(
			"expected --json NoOptDefVal=%q, got %q",
			connect.JSONFormatPretty,
			jsonFlag.NoOptDefVal,
		)
	}
	if connectCmd.LocalNonPersistentFlags().Lookup("json-format") != nil {
		t.Fatal("did not expect --json-format to remain registered")
	}
}
