// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/presets"
	"github.com/exasol/exasol-personal/internal/runtimeartifacts"
	"github.com/exasol/exasol-personal/internal/tofu"
	"github.com/zclconf/go-cty/cty"
)

const tofuResourceID = "tofu"

// writeFakeTofuManager builds a Manager whose "tofu" resource resolves to a
// fake tofu/OpenTofu executable, so tofuBackend.Deploy/Destroy/Stop exercise
// their real init/plan/apply/destroy wiring without needing the real binary.
func writeFakeTofuManager(t *testing.T, scriptContent string) *runtimeartifacts.Manager {
	t.Helper()

	zipPath := filepath.Join(t.TempDir(), "tofu.zip")
	file, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("failed to create tofu zip fixture: %v", err)
	}
	defer file.Close()

	writer := zip.NewWriter(file)
	header := &zip.FileHeader{Name: "tofu", Method: zip.Deflate}
	header.SetMode(testRunnerExecutableMode)
	entry, err := writer.CreateHeader(header)
	if err != nil {
		t.Fatalf("failed to create tofu zip entry: %v", err)
	}
	if _, err := entry.Write([]byte(scriptContent)); err != nil {
		t.Fatalf("failed to write tofu zip entry: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close tofu zip fixture: %v", err)
	}

	spec := runtimeartifacts.ResourceSpec{
		tofuResourceID: {
			Extract: true,
			Artifact: map[string]runtimeartifacts.ArtifactSpec{
				"any": {URL: zipPath, ResourcePath: "tofu"},
			},
		},
	}

	return runtimeartifacts.NewResourceManagerForPlatform(
		spec,
		t.TempDir(),
		runtime.GOOS,
		runtime.GOARCH,
	)
}

// newTestTofuBackend builds a tofuBackend against a real deployment directory
// with a minimal variables file, backed by a fake tofu executable.
func newTestTofuBackend(t *testing.T, tofuScript string) *tofuBackend {
	t.Helper()

	deployment := config.NewDeploymentDir(t.TempDir())
	manifest := &presets.InfrastructureManifest{
		Backend: backendTypeTofu,
		Tofu:    &presets.InfrastructureTofu{},
	}
	manager := writeFakeTofuManager(t, tofuScript)

	backend := newTofuBackend(deployment, manifest, manager)

	if err := os.MkdirAll(backend.cfg.WorkDir(), 0o750); err != nil {
		t.Fatalf("failed to create infrastructure dir: %v", err)
	}
	variablesTF := `variable "region" {
  type    = string
  default = "us-east-1"
}
`
	if err := os.WriteFile(backend.cfg.VariablesFile(), []byte(variablesTF), 0o600); err != nil {
		t.Fatalf("failed to write variables file: %v", err)
	}

	return backend
}

const alwaysSucceedTofuScript = "#!/bin/sh\nexit 0\n"

func failOnSubcommandTofuScript(subcommand string) string {
	return "#!/bin/sh\nif [ \"$1\" = \"" + subcommand + "\" ]; then exit 1; fi\nexit 0\n"
}

// nolint: paralleltest // avoids concurrent extract+exec of the fake tofu binary (ETXTBSY flakes).
func TestTofuBackendDeploySucceedsThroughInitPlanApply(t *testing.T) {
	backend := newTestTofuBackend(t, alwaysSucceedTofuScript)

	var out, outErr bytes.Buffer

	err := backend.Deploy(context.Background(), &out, &outErr, DeployOptions{})
	if err != nil {
		t.Fatalf("expected deploy to succeed, got %v", err)
	}
}

// nolint: paralleltest // avoids concurrent extract+exec of the fake tofu binary (ETXTBSY flakes).
func TestTofuBackendDeployStopsAtPlanFailureWithoutApplying(t *testing.T) {
	backend := newTestTofuBackend(t, failOnSubcommandTofuScript("plan"))

	var out, outErr bytes.Buffer

	err := backend.Deploy(context.Background(), &out, &outErr, DeployOptions{})
	if err == nil {
		t.Fatal("expected deploy to fail when plan fails")
	}
}

func TestTofuBackendDeployIsANoopWithoutTofuConfiguration(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())
	backend := newTofuBackend(deployment, &presets.InfrastructureManifest{Backend: "local"}, nil)

	var out, outErr bytes.Buffer

	err := backend.Deploy(context.Background(), &out, &outErr, DeployOptions{})
	if err != nil {
		t.Fatalf("expected a no-op deploy without tofu config, got %v", err)
	}
}

// nolint: paralleltest // avoids concurrent extract+exec of the fake tofu binary (ETXTBSY flakes).
func TestTofuBackendDestroySucceedsThroughInitAndDestroy(t *testing.T) {
	backend := newTestTofuBackend(t, alwaysSucceedTofuScript)

	var out, outErr bytes.Buffer

	err := backend.Destroy(context.Background(), &out, &outErr)
	if err != nil {
		t.Fatalf("expected destroy to succeed, got %v", err)
	}
}

// nolint: paralleltest // avoids concurrent extract+exec of the fake tofu binary (ETXTBSY flakes).
func TestTofuBackendDestroyPropagatesDestroyFailure(t *testing.T) {
	backend := newTestTofuBackend(t, failOnSubcommandTofuScript("destroy"))

	var out, outErr bytes.Buffer

	err := backend.Destroy(context.Background(), &out, &outErr)
	if err == nil {
		t.Fatal("expected destroy to fail when the destroy step fails")
	}
}

func TestTofuBackendDestroyIsANoopWithoutTofuConfiguration(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())
	backend := newTofuBackend(deployment, &presets.InfrastructureManifest{Backend: "local"}, nil)

	var out, outErr bytes.Buffer
	if err := backend.Destroy(context.Background(), &out, &outErr); err != nil {
		t.Fatalf("expected a no-op destroy without tofu config, got %v", err)
	}
}

// nolint: paralleltest // avoids concurrent extract+exec of the fake tofu binary (ETXTBSY flakes).
func TestTofuBackendStopAppliesStoppedPowerState(t *testing.T) {
	backend := newTestTofuBackend(t, alwaysSucceedTofuScript)

	var out, outErr bytes.Buffer
	if err := backend.Stop(context.Background(), &out, &outErr); err != nil {
		t.Fatalf("expected stop to succeed, got %v", err)
	}
}

// nolint: paralleltest // avoids concurrent extract+exec of the fake tofu binary (ETXTBSY flakes).
func TestTofuBackendStopPropagatesApplyFailure(t *testing.T) {
	backend := newTestTofuBackend(t, failOnSubcommandTofuScript("apply"))

	var out, outErr bytes.Buffer

	err := backend.Stop(context.Background(), &out, &outErr)
	if err == nil {
		t.Fatal("expected stop to fail when tofu apply fails")
	}
}

func TestTofuBackendApplyActionIsANoopWithoutTofuConfiguration(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())
	backend := newTofuBackend(deployment, &presets.InfrastructureManifest{Backend: "local"}, nil)

	var out, outErr bytes.Buffer

	err := backend.applyAction(context.Background(), "power_state=stopped", &out, &outErr)
	if err != nil {
		t.Fatalf("expected a no-op apply action without tofu config, got %v", err)
	}
}

func TestTofuBackendValidateEnvironmentAndSetupWorkspaceAreNoops(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())
	backend := newTofuBackend(deployment, &presets.InfrastructureManifest{Backend: "local"}, nil)

	if err := backend.ValidateEnvironment(); err != nil {
		t.Fatalf("expected ValidateEnvironment to be a no-op, got %v", err)
	}
	if err := backend.SetupWorkspace(context.Background()); err != nil {
		t.Fatalf("expected SetupWorkspace to be a no-op without tofu config, got %v", err)
	}
}

func TestTofuBackendReadConfigurationIsEmptyWithoutTofuConfiguration(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())
	backend := newTofuBackend(deployment, &presets.InfrastructureManifest{Backend: "local"}, nil)

	values, err := backend.ReadConfiguration()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(values) != 0 {
		t.Fatalf("expected no configuration values without tofu config, got %+v", values)
	}
}

// newTestTofuBackendNoBinary builds a tofuBackend for read-only paths
// (ReadConfiguration/ReadDeploymentConfigVariables) that never need to
// resolve or exec the tofu binary itself.
func newTestTofuBackendNoBinary(t *testing.T, variablesTF string) *tofuBackend {
	t.Helper()

	deployment := config.NewDeploymentDir(t.TempDir())
	manifest := &presets.InfrastructureManifest{
		Backend: backendTypeTofu,
		Tofu:    &presets.InfrastructureTofu{},
	}
	backend := newTofuBackend(deployment, manifest, nil)

	if err := os.MkdirAll(backend.cfg.WorkDir(), 0o750); err != nil {
		t.Fatalf("failed to create infrastructure dir: %v", err)
	}
	if err := os.WriteFile(backend.cfg.VariablesFile(), []byte(variablesTF), 0o600); err != nil {
		t.Fatalf("failed to write variables file: %v", err)
	}

	return backend
}

const reservedNameVariablesTF = `variable "region" {
  type    = string
  default = "us-east-1"
}

variable "deployment_id" {
  type    = string
  default = ""
}
`

func TestTofuBackendReadDeploymentConfigVariablesExcludesReservedNames(t *testing.T) {
	t.Parallel()

	backend := newTestTofuBackendNoBinary(t, reservedNameVariablesTF)

	variables, err := backend.ReadDeploymentConfigVariables()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if _, ok := variables["region"]; !ok {
		t.Fatalf("expected 'region' to be exposed, got %+v", variables)
	}
	if _, ok := variables["deployment_id"]; ok {
		t.Fatalf("expected launcher-reserved 'deployment_id' to be excluded, got %+v", variables)
	}
}

func TestTofuBackendReadConfigurationReflectsWrittenOverrides(t *testing.T) {
	t.Parallel()

	backend := newTestTofuBackendNoBinary(t, reservedNameVariablesTF)

	tfvars := "region = \"eu-central-1\"\n"
	if err := os.WriteFile(backend.cfg.VarsOutputFile(), []byte(tfvars), 0o600); err != nil {
		t.Fatalf("failed to write vars output file: %v", err)
	}

	values, err := backend.ReadConfiguration()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	var region *DeploymentConfigValue
	for i := range values {
		if values[i].Name == "region" {
			region = &values[i]
		}
		if values[i].Name == "deployment_id" {
			t.Fatalf("expected reserved variable to be excluded, got %+v", values[i])
		}
	}
	if region == nil {
		t.Fatal("expected a 'region' configuration value")
	}
	if region.Value != "eu-central-1" || region.Default != "us-east-1" {
		t.Fatalf("expected current override and preset default, got %+v", region)
	}
}

func TestReadTofuPresetConfigVariablesFromPathPreset(t *testing.T) {
	t.Parallel()

	presetDir := t.TempDir()
	tofuManifest := presets.InfrastructureTofu{}
	tofuCfg := tofu.NewTofuConfigFromPreset(presetDir, tofuManifest)
	if err := os.MkdirAll(presetDir, 0o750); err != nil {
		t.Fatalf("failed to create preset dir: %v", err)
	}
	variablesData := []byte(reservedNameVariablesTF)
	if err := os.WriteFile(tofuCfg.VariablesFile(), variablesData, 0o600); err != nil {
		t.Fatalf("failed to write variables file: %v", err)
	}

	variables, err := readTofuPresetConfigVariables(PresetRef{Path: presetDir}, tofuManifest)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if _, ok := variables["region"]; !ok {
		t.Fatalf("expected 'region' to be exposed, got %+v", variables)
	}
	if _, ok := variables["deployment_id"]; ok {
		t.Fatalf("expected reserved 'deployment_id' to be excluded, got %+v", variables)
	}
}

func TestTofuVariableDefinitionsFiltersReservedAndNilVariables(t *testing.T) {
	t.Parallel()

	definitions := tofuVariableDefinitions(map[string]*tofu.Variable{
		"region":         {Type: "string", Value: cty.StringVal("us-east-1")},
		"deployment_id":  {Type: "string", Value: cty.StringVal("")},
		"nil-entry-safe": nil,
	})

	if len(definitions) != 1 {
		t.Fatalf("expected exactly one exposed variable, got %+v", definitions)
	}
	if _, ok := definitions["region"]; !ok {
		t.Fatalf("expected 'region' to be exposed, got %+v", definitions)
	}
}

func TestCtyDefaultDisplayHandlesNullUnknownAndConcreteValues(t *testing.T) {
	t.Parallel()

	if got := ctyDefaultDisplay(cty.NullVal(cty.String)); got != "" {
		t.Fatalf("expected empty display for a null value, got %q", got)
	}
	if got := ctyDefaultDisplay(cty.UnknownVal(cty.String)); got != "" {
		t.Fatalf("expected empty display for an unknown value, got %q", got)
	}
	if got := ctyDefaultDisplay(cty.StringVal("us-east-1")); got != `"us-east-1"` {
		t.Fatalf("expected JSON-encoded string, got %q", got)
	}
}

func TestTofuBackendOpenHostShellPropagatesSSHResolutionError(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())
	if err := config.WriteDeploymentInfo(deployment.Root(), &config.DeploymentInfo{
		Backend:      backendTypeTofu,
		DeploymentId: "dep-1",
	}); err != nil {
		t.Fatalf("failed to write deployment info: %v", err)
	}
	manifest := &presets.InfrastructureManifest{Backend: backendTypeTofu}
	backend := newTofuBackend(deployment, manifest, nil)

	err := backend.OpenHostShell(context.Background(), "")

	if !errors.Is(err, ErrNoNodesFound) {
		t.Fatalf("expected ErrNoNodesFound, got %v", err)
	}
}

func TestTofuBackendOpenCOSShellPropagatesSSHResolutionError(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())
	if err := config.WriteDeploymentInfo(deployment.Root(), &config.DeploymentInfo{
		Backend:      backendTypeTofu,
		DeploymentId: "dep-1",
	}); err != nil {
		t.Fatalf("failed to write deployment info: %v", err)
	}
	manifest := &presets.InfrastructureManifest{Backend: backendTypeTofu}
	backend := newTofuBackend(deployment, manifest, nil)

	err := backend.OpenCOSShell(context.Background())

	if !errors.Is(err, config.ErrUnknownNodeName) {
		t.Fatalf("expected ErrUnknownNodeName (no n11 node present), got %v", err)
	}
}

func TestCtyScalarConversionsRejectUnknownOrNullValues(t *testing.T) {
	t.Parallel()

	unknown := cty.UnknownVal(cty.String)
	if _, err := ctyScalarToRawString(unknown); err == nil {
		t.Fatal("expected ctyScalarToRawString to reject an unknown value")
	}
	if _, err := ctyScalarToGoValue(unknown); err == nil {
		t.Fatal("expected ctyScalarToGoValue to reject an unknown value")
	}

	null := cty.NullVal(cty.String)
	if _, err := ctyScalarToRawString(null); err == nil {
		t.Fatal("expected ctyScalarToRawString to reject a null value")
	}
	if _, err := ctyScalarToGoValue(null); err == nil {
		t.Fatal("expected ctyScalarToGoValue to reject a null value")
	}
}
