// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/exasol/exasol-personal/internal/presets"
)

func TestWriteInstallationVariablesFile_NilSpecIsNoop(t *testing.T) {
	t.Parallel()

	err := writeInstallationVariablesFile(t.TempDir(), nil, "id", "cid", "url", nil)
	if err != nil {
		t.Fatalf("expected a nil spec to be a no-op, got %v", err)
	}
}

func TestWriteInstallationVariablesFile_BlankOutputFileIsNoop(t *testing.T) {
	t.Parallel()

	spec := &presets.Variables{OutputFile: "   "}

	err := writeInstallationVariablesFile(t.TempDir(), spec, "id", "cid", "url", nil)
	if err != nil {
		t.Fatalf("expected a blank outputFile to be a no-op, got %v", err)
	}
}

func TestWriteInstallationVariablesFile_RejectsPathEscapingInstallDir(t *testing.T) {
	t.Parallel()

	spec := &presets.Variables{OutputFile: "../escape.json"}

	err := writeInstallationVariablesFile(t.TempDir(), spec, "id", "cid", "url", nil)
	if err == nil {
		t.Fatal("expected an error when outputFile escapes the installation directory")
	}
}

func TestWriteInstallationVariablesFile_AppliesDefaultsAndGovernedIdentity(t *testing.T) {
	t.Parallel()

	installDir := t.TempDir()
	spec := &presets.Variables{
		OutputFile: "resolved/values.json",
		Vars: map[string]*presets.VariableDef{
			"admin_password": {Type: "string", Default: "changeit"},
		},
	}

	err := writeInstallationVariablesFile(
		installDir, spec, "exasol-personal;abcd1234", "abcd1234", "https://example.test", nil,
	)
	if err != nil {
		t.Fatalf("expected write to succeed, got %v", err)
	}

	resolved := readResolvedInstallationVariables(
		t,
		filepath.Join(installDir, "resolved/values.json"),
	)
	if resolved["deployment_id"] != "abcd1234" {
		t.Fatalf("expected governed deployment_id, got %+v", resolved)
	}
	if resolved["cluster_identity"] != "exasol-personal;abcd1234" {
		t.Fatalf("expected governed cluster_identity, got %+v", resolved)
	}
	if resolved["version_check_url"] != "https://example.test" {
		t.Fatalf("expected governed version_check_url, got %+v", resolved)
	}
	if resolved["admin_password"] != "changeit" {
		t.Fatalf("expected the manifest default to be applied, got %+v", resolved)
	}
}

func TestWriteInstallationVariablesFile_OverridesReplaceDefaultsAndIgnoreUnknownNames(
	t *testing.T,
) {
	t.Parallel()

	installDir := t.TempDir()
	spec := &presets.Variables{
		OutputFile: "values.json",
		Vars: map[string]*presets.VariableDef{
			"cluster_size": {Type: "number", Default: 1},
		},
	}

	err := writeInstallationVariablesFile(
		installDir, spec, "id", "cid", "url",
		map[string]string{"cluster_size": "3", "does_not_exist": "ignored"},
	)
	if err != nil {
		t.Fatalf("expected write to succeed, got %v", err)
	}

	resolved := readResolvedInstallationVariables(t, filepath.Join(installDir, "values.json"))
	if resolved["cluster_size"] != float64(3) {
		t.Fatalf("expected the override to replace the default, got %+v", resolved)
	}
	if _, present := resolved["does_not_exist"]; present {
		t.Fatalf("expected an override for an unknown variable to be ignored, got %+v", resolved)
	}
}

func TestWriteInstallationVariablesFile_ReservedNamesCannotBeOverridden(t *testing.T) {
	t.Parallel()

	installDir := t.TempDir()
	spec := &presets.Variables{
		OutputFile: "values.json",
		Vars: map[string]*presets.VariableDef{
			"deployment_id": {Type: "string", Default: "manifest-default"},
		},
	}

	err := writeInstallationVariablesFile(
		installDir, spec, "cid", "governed-id", "url",
		map[string]string{"deployment_id": "attempted-override"},
	)
	if err != nil {
		t.Fatalf("expected write to succeed, got %v", err)
	}

	resolved := readResolvedInstallationVariables(t, filepath.Join(installDir, "values.json"))
	if resolved["deployment_id"] != "governed-id" {
		t.Fatalf("expected the governed identity to win, got %+v", resolved)
	}
}

func TestWriteInstallationVariablesFile_InvalidManifestDefaultReturnsError(t *testing.T) {
	t.Parallel()

	spec := &presets.Variables{
		OutputFile: "values.json",
		Vars: map[string]*presets.VariableDef{
			"bad": {Type: "string", Default: []string{"unsupported"}},
		},
	}

	err := writeInstallationVariablesFile(t.TempDir(), spec, "id", "cid", "url", nil)
	if err == nil {
		t.Fatal("expected an error for an unsupported manifest default type")
	}
}

func TestWriteInstallationVariablesFile_InvalidOverrideValueReturnsError(t *testing.T) {
	t.Parallel()

	spec := &presets.Variables{
		OutputFile: "values.json",
		Vars: map[string]*presets.VariableDef{
			"cluster_size": {Type: "number", Default: 1},
		},
	}

	err := writeInstallationVariablesFile(
		t.TempDir(), spec, "id", "cid", "url", map[string]string{"cluster_size": "not-a-number"},
	)
	if err == nil {
		t.Fatal("expected an error for an override that doesn't match its declared type")
	}
}

func readResolvedInstallationVariables(t *testing.T, path string) map[string]any {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read resolved installation variables: %v", err)
	}

	var resolved map[string]any
	if err := json.Unmarshal(data, &resolved); err != nil {
		t.Fatalf("failed to decode resolved installation variables: %v", err)
	}

	return resolved
}

func TestIsReservedInstallationVariableName(t *testing.T) {
	t.Parallel()

	cases := map[string]bool{
		"deployment_id":     true,
		"cluster_identity":  true,
		"version_check_url": true,
		"admin_password":    false,
		"":                  false,
	}
	for name, want := range cases {
		if got := isReservedInstallationVariableName(name); got != want {
			t.Errorf("isReservedInstallationVariableName(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestParseInstallVarValue(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		varType string
		raw     string
		want    any
		wantErr bool
	}{
		"bool true":         {varType: "bool", raw: "true", want: true},
		"bool invalid":      {varType: "bool", raw: "nope", wantErr: true},
		"number":            {varType: "number", raw: "3.5", want: 3.5},
		"number invalid":    {varType: "number", raw: "nope", wantErr: true},
		"string explicit":   {varType: "string", raw: "hello", want: "hello"},
		"string blank type": {varType: "", raw: "hello", want: "hello"},
		"unsupported type":  {varType: "list", raw: "x", wantErr: true},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := parseInstallVarValue(testCase.varType, testCase.raw)
			if testCase.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}

				return
			}
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if got != testCase.want {
				t.Fatalf("expected %v, got %v", testCase.want, got)
			}
		})
	}
}
