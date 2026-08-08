// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package presets

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

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

func TestReadInstallManifestFromDir_MissingFileReturnsWrappedError(t *testing.T) {
	t.Parallel()

	// Given / When
	_, err := ReadInstallManifestFromDir(t.TempDir())
	// Then
	if err == nil {
		t.Fatal("expected an error for a missing manifest file")
	}
}

func TestReadInstallManifestFromDir_ParsesRealManifestFile(t *testing.T) {
	t.Parallel()

	// Given
	dir := t.TempDir()
	manifestYAML := "name: Test Install\ndescription: test install\n"
	path := filepath.Join(dir, InstallationManifestFilename)
	if err := os.WriteFile(path, []byte(manifestYAML), 0o600); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	// When
	manifest, err := ReadInstallManifestFromDir(dir)
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if manifest.Name != "Test Install" {
		t.Fatalf("expected manifest name to be parsed, got %q", manifest.Name)
	}
}

func TestReadInstallManifest_ReadsRealEmbeddedLocalPreset(t *testing.T) {
	t.Parallel()

	// Given / When
	manifest, err := ReadInstallManifest("local")
	// Then
	if err != nil {
		t.Fatalf("expected the embedded 'local' install preset to load, got %v", err)
	}
	if manifest.Name == "" {
		t.Fatal("expected the embedded manifest to declare a name")
	}
}

func TestReadInstallManifest_UnknownPresetReturnsError(t *testing.T) {
	t.Parallel()

	// Given / When
	_, err := ReadInstallManifest("does-not-exist")
	// Then
	if err == nil {
		t.Fatal("expected an error for an unknown installation preset")
	}
}

func TestVariableDefDefaultScalar(t *testing.T) {
	t.Parallel()

	// Given
	cases := map[string]struct {
		def     *VariableDef
		wantErr error
	}{
		"nil definition":  {def: nil, wantErr: ErrInvalidInstallationVariable},
		"missing default": {def: &VariableDef{}, wantErr: ErrInvalidInstallationVariable},
		"unsupported type": {
			def:     &VariableDef{Default: []string{"a"}},
			wantErr: ErrInvalidInstallationVariable,
		},
		"valid string default": {def: &VariableDef{Default: "value"}},
		"valid bool default":   {def: &VariableDef{Default: true}},
		"valid number default": {def: &VariableDef{Default: 3}},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// When
			value, err := testCase.def.DefaultScalar()

			// Then
			if testCase.wantErr != nil {
				if !errors.Is(err, testCase.wantErr) {
					t.Fatalf("expected %v, got %v", testCase.wantErr, err)
				}

				return
			}
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if value != testCase.def.Default {
				t.Fatalf("expected default value %v, got %v", testCase.def.Default, value)
			}
		})
	}
}

func TestVariableDefEffectiveType(t *testing.T) {
	t.Parallel()

	// Given
	cases := map[string]struct {
		def     *VariableDef
		want    string
		wantErr bool
	}{
		"explicit valid type":   {def: &VariableDef{Type: "bool", Default: true}, want: "bool"},
		"explicit invalid type": {def: &VariableDef{Type: "object"}, wantErr: true},
		"inferred from bool":    {def: &VariableDef{Default: true}, want: "bool"},
		"inferred from number":  {def: &VariableDef{Default: 3}, want: "number"},
		"inferred from string":  {def: &VariableDef{Default: "x"}, want: "string"},
		"nil definition":        {def: nil, wantErr: true},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// When
			got, err := testCase.def.EffectiveType()

			// Then
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
				t.Fatalf("expected %q, got %q", testCase.want, got)
			}
		})
	}
}

func TestInstallStepUnmarshalYAML_FlattenedStyleIsTreatedAsRemoteExec(t *testing.T) {
	t.Parallel()

	// Given
	manifestRaw := []byte(
		"name: Test Install\n" +
			"description: test install\n" +
			"install:\n" +
			"  - description: run remotely\n" +
			"    filename: monitor.sh\n",
	)

	// When
	manifest, err := parseInstallManifest(manifestRaw)
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(manifest.Install) != 1 || manifest.Install[0].RemoteExec == nil {
		t.Fatalf(
			"expected the flattened step to be treated as remoteExec, got %+v",
			manifest.Install,
		)
	}
	if manifest.Install[0].RemoteExec.Filename != "monitor.sh" {
		t.Fatalf("expected filename to be parsed, got %+v", manifest.Install[0].RemoteExec)
	}
}

func TestInstallStepUnmarshalYAML_RejectsBothTaskTypesSet(t *testing.T) {
	t.Parallel()

	// Given
	manifestRaw := []byte(
		"name: Test Install\n" +
			"description: test install\n" +
			"install:\n" +
			"  - remoteExec:\n" +
			"      filename: monitor.sh\n" +
			"    localCommand:\n" +
			"      command: [\"echo\", \"hi\"]\n",
	)

	// When
	_, err := parseInstallManifest(manifestRaw)
	// Then
	if err == nil {
		t.Fatal("expected an error when both task types are set on one step")
	}
}

func TestInstallStepsUnmarshalYAML_WrapsSingleStepObject(t *testing.T) {
	t.Parallel()

	// Given
	manifestRaw := []byte(
		"name: Test Install\n" +
			"description: test install\n" +
			"install:\n" +
			"  remoteExec:\n" +
			"    description: run remotely\n" +
			"    filename: monitor.sh\n",
	)

	// When
	manifest, err := parseInstallManifest(manifestRaw)
	// Then
	if err != nil {
		t.Fatalf("expected a single step object to be wrapped, got %v", err)
	}
	if len(manifest.Install) != 1 || manifest.Install[0].RemoteExec == nil {
		t.Fatalf("expected exactly one wrapped remoteExec step, got %+v", manifest.Install)
	}
}
