// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package presets

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/exasol/exasol-personal/assets"
	"gopkg.in/yaml.v3"
)

// InstallManifest represents the installation preset workflow and metadata.
// It is read from <deploymentDir>/installation/installation.yaml (extracted from assets).
type InstallManifest struct {
	Name        string       `yaml:"name"`
	Description string       `yaml:"description"`
	Variables   *Variables   `yaml:"variables"`
	Install     InstallSteps `yaml:"install"`
}

// Variables defines installation-preset-owned variables.
//
// These variables are exposed as CLI flags by the launcher and materialized into
// a resolved-values file whose path is defined by OutputFile.
type Variables struct {
	// OutputFile is a path relative to <deploymentDir>/installation/.
	OutputFile string                  `yaml:"outputFile"`
	Vars       map[string]*VariableDef `yaml:"vars"`
}

// VariableDef describes a single installation variable.
//
// Type is intentionally small (string/bool/number) to keep CLI parsing and JSON
// materialization straightforward.
type VariableDef struct {
	Description string `yaml:"description"`
	Type        string `yaml:"type"`
	Default     any    `yaml:"default"`
}

var ErrInvalidInstallationVariable = errors.New("invalid installation variable")

// DefaultScalar returns the default value and ensures it is a scalar type suitable for
// CLI flags and JSON materialization.
//
// Design decision: installation variables intentionally support only primitive scalars.
func (d *VariableDef) DefaultScalar() (any, error) {
	if d == nil {
		return nil, fmt.Errorf("%w: nil definition", ErrInvalidInstallationVariable)
	}
	if d.Default == nil {
		return nil, fmt.Errorf("%w: missing default", ErrInvalidInstallationVariable)
	}

	switch d.Default.(type) {
	case string, bool, int, int64, float64:
		return d.Default, nil
	default:
		return nil, fmt.Errorf(
			"%w: unsupported default type %T (only string/bool/number allowed)",
			ErrInvalidInstallationVariable,
			d.Default,
		)
	}
}

// EffectiveType returns the variable type used for CLI parsing.
//
// If the manifest provides an explicit type, we use it (and validate it's one of the
// supported primitives). Otherwise we infer the type from the YAML default value.
func (d *VariableDef) EffectiveType() (string, error) {
	if d == nil {
		return "", fmt.Errorf("%w: nil definition", ErrInvalidInstallationVariable)
	}
	if t := strings.TrimSpace(d.Type); t != "" {
		switch t {
		case "string", "bool", "number":
			return t, nil
		default:
			return "", fmt.Errorf("%w: unsupported type %q", ErrInvalidInstallationVariable, t)
		}
	}

	def, err := d.DefaultScalar()
	if err != nil {
		return "", err
	}
	switch def.(type) {
	case bool:
		return "bool", nil
	case int, int64, float64:
		return "number", nil
	case string:
		return "string", nil
	default:
		// DefaultScalar should have filtered this already.
		return "", fmt.Errorf("%w: cannot infer type from %T", ErrInvalidInstallationVariable, def)
	}
}

// InstallStep supports remoteExec tasks.
type InstallStep struct {
	RemoteExec *RemoteExecTask `yaml:"remoteExec"`
}

// UnmarshalYAML allows InstallStep to be defined in two YAML styles:
// 1) Explicit nested style:
//   - remoteExec:
//     description: ...
//     filename: ...
//
// 2) Implicit flattened style (fields at the top-level of the step):
//   - description: ...
//     filename: ...
//
// In the flattened style, the step is treated as a remoteExec step.
func (s *InstallStep) UnmarshalYAML(value *yaml.Node) error {
	// Decode into the struct as-is first (nested remoteExec style)
	type alias InstallStep
	var tmp alias
	if err := value.Decode(&tmp); err != nil {
		return err
	}
	*s = InstallStep(tmp)

	if s.RemoteExec != nil {
		return nil
	}

	// If remoteExec was not set, try decoding the entire node into RemoteExecTask
	var rex RemoteExecTask
	if err := value.Decode(&rex); err == nil {
		// Heuristics: consider it a remoteExec step only if at least one key is populated
		if rex.Description != "" || rex.Filename != "" || rex.Node != "" || len(rex.RegexLog) > 0 {
			s.RemoteExec = &rex
			return nil
		}
	}

	// If still nothing, leave as empty; higher-level validation will handle it
	return nil
}

// InstallSteps is a list of InstallStep with a flexible YAML parser:
// - accepts a proper YAML sequence of steps
// - accepts a single step object (mapping) and wraps it as a one-item list
// This makes the loader resilient if manifests provide a single object under "install".
type InstallSteps []InstallStep

func (s *InstallSteps) UnmarshalYAML(unmarshal func(any) error) error {
	// First, try to parse as a sequence of steps
	var seq []InstallStep
	if err := unmarshal(&seq); err == nil {
		*s = seq
		return nil
	}

	// Next, try to parse as a single step object and wrap it
	var single InstallStep
	if err := unmarshal(&single); err == nil {
		*s = []InstallStep{single}
		return nil
	}

	// As a fallback, try to parse arbitrary mapping and re-decode into InstallStep
	var raw map[string]any
	if err := unmarshal(&raw); err == nil {
		// Attempt to marshal back and unmarshal into InstallStep
		data, err2 := yaml.Marshal(raw)
		if err2 == nil {
			var ss InstallStep
			if err3 := yaml.Unmarshal(data, &ss); err3 == nil {
				*s = []InstallStep{ss}
				return nil
			}
		}
	}

	return errors.New("install must be a list of steps or a single step object")
}

// RegexLog defines a regex pattern and the message emitted when it matches.
type RegexLog struct {
	Regex         string         `yaml:"regex"`
	Message       string         `yaml:"message"`
	LogAsError    bool           `yaml:"logAsError"`
	CompiledRegex *regexp.Regexp `yaml:"-"`
}

func (s *RegexLog) UnmarshalYAML(value *yaml.Node) error {
	type alias RegexLog
	var tmp alias

	if err := value.Decode(&tmp); err != nil {
		return err
	}

	*s = RegexLog(tmp)

	compiled, err := regexp.Compile(s.Regex)
	if err != nil {
		return err
	}

	s.CompiledRegex = compiled

	return nil
}

// RemoteExecTask describes a remote script execution task.
type RemoteExecTask struct {
	Description       string      `yaml:"description"`
	Filename          string      `yaml:"filename"`
	ExecuteInParallel bool        `yaml:"executeInParallel"`
	Node              string      `yaml:"node"`
	RegexLog          []*RegexLog `yaml:"regexLog"`
}

// LocalCommandTask describes a local command task.
type LocalCommandTask struct {
	Description string      `yaml:"description"`
	Command     []string    `yaml:"command"`
	Node        string      `yaml:"node"`
	RegexLog    []*RegexLog `yaml:"regexLog"`
}

// ReadInstallManifest loads the installation manifest from embedded assets.
func ReadInstallManifest(installName string) (*InstallManifest, error) {
	manifestRaw, err := assets.InstallationAssets.ReadFile(
		assets.InstallationAssetDir + "/" + installName + "/" + InstallationManifestFilename,
	)
	if err != nil {
		return nil, err
	}

	return parseInstallManifest(manifestRaw)
}

// ReadInstallManifestFromDir loads the installation manifest from a preset directory on disk.
func ReadInstallManifestFromDir(dir string) (*InstallManifest, error) {
	manifestRaw, err := os.ReadFile(filepath.Join(dir, InstallationManifestFilename))
	if err != nil {
		return nil, fmt.Errorf("failed to read installation manifest from %q: %w", dir, err)
	}

	return parseInstallManifest(manifestRaw)
}

func parseInstallManifest(manifestRaw []byte) (*InstallManifest, error) {
	var manifest InstallManifest

	decoder := yaml.NewDecoder(bytes.NewReader(manifestRaw))
	// Do not enforce KnownFields to allow future/unknown keys.
	err := decoder.Decode(&manifest)
	if err != nil {
		return nil, err
	}

	return &manifest, nil
}
