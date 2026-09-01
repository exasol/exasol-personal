// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package presets

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/resource"
	"github.com/exasol/exasol-personal/internal/resource/resourcetest"
)

func testResolverContext(t *testing.T, spec resource.ResourceSpec) context.Context {
	t.Helper()

	return resourcetest.NewResolverContext(t, spec)
}

func TestResourceContextWithoutSpecificationKeepsLauncherResources(t *testing.T) {
	t.Parallel()

	// Given
	presetDir := t.TempDir()
	resourceDir := t.TempDir()
	ctx := testResolverContext(t, resource.ResourceSpec{
		"tool": {Artifact: map[string]resource.ArtifactSpec{
			"any": {URL: "file://" + resourceDir},
		}},
	})

	// When
	layered, resolvedDir, err := ResourceContext(ctx, Infrastructure, PresetRef{Path: presetDir})
	var resolvedResource string
	if err == nil {
		resolvedResource, err = resource.FromContext(layered).Resolve(layered, "tool")
	}

	// Then
	if err != nil {
		t.Fatalf("resolve launcher resource: %v", err)
	}
	if resolvedDir != presetDir {
		t.Fatalf("preset directory = %q, want %q", resolvedDir, presetDir)
	}
	if resolvedResource != resourceDir {
		t.Fatalf("resource = %q, want %q", resolvedResource, resourceDir)
	}
}

func TestResourceContextReportsInvalidSpecificationWithPresetName(t *testing.T) {
	t.Parallel()

	// Given
	presetDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(presetDir, resourceSpecFilename), []byte("tool: ["), 0o600,
	); err != nil {
		t.Fatalf("write resource specification: %v", err)
	}
	ctx := testResolverContext(t, resource.ResourceSpec{})

	// When
	_, _, err := ResourceContext(ctx, Infrastructure, PresetRef{Path: presetDir})

	// Then
	if err == nil || !strings.Contains(err.Error(), "invalid resource specification") ||
		!strings.Contains(err.Error(), presetDir) {
		t.Fatalf("error = %v, want invalid specification and preset name", err)
	}
}

//nolint:paralleltest // The process-wide logger must capture each directive warning.
func TestResourceContextIgnoresBuildDirectivesWithWarnings(t *testing.T) {
	for _, directive := range []struct {
		name  string
		value string
	}{
		{name: "embed", value: "true"},
		{name: "glob", value: `"*"`},
	} {
		t.Run(directive.name, func(t *testing.T) {
			// Given
			presetDir := t.TempDir()
			resourceDir := t.TempDir()
			spec := fmt.Sprintf(
				"tool:\n  %s: %s\n  artifact:\n    any:\n      url: file://%s\n",
				directive.name, directive.value, resourceDir,
			)
			if err := os.WriteFile(
				filepath.Join(presetDir, resourceSpecFilename), []byte(spec), 0o600,
			); err != nil {
				t.Fatalf("write resource specification: %v", err)
			}
			var logs bytes.Buffer
			originalLogger := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
			defer slog.SetDefault(originalLogger)
			ctx := testResolverContext(t, resource.ResourceSpec{})

			// When
			layered, _, err := ResourceContext(ctx, Infrastructure, PresetRef{Path: presetDir})
			if err == nil {
				_, err = resource.FromContext(layered).Resolve(layered, "tool")
			}

			// Then
			if err != nil {
				t.Fatalf("resolve resource: %v", err)
			}
			if logText := logs.String(); !strings.Contains(logText, "directive="+directive.name) ||
				!strings.Contains(logText, presetDir) {
				t.Fatalf("warning = %q, want directive and preset", logText)
			}
		})
	}
}

func TestPresetDir_UnknownInfrastructureNameReturnsErrUnknownInfrastructure(t *testing.T) {
	t.Parallel()

	// Given
	ctx := testResolverContext(t, resource.ResourceSpec{})

	// When
	_, err := presetDir(ctx, Infrastructure, "this-preset-does-not-exist")
	// Then
	if !errors.Is(err, ErrUnknownInfrastructure) {
		t.Fatalf("expected ErrUnknownInfrastructure, got %v", err)
	}
}

func TestResourceContextScopesPresetResources(t *testing.T) {
	t.Parallel()

	// Given
	presetDir := t.TempDir()
	resourceDir := filepath.Join(presetDir, "resource")
	if err := os.Mkdir(resourceDir, 0o700); err != nil {
		t.Fatalf("create resource directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(presetDir, resourceSpecFilename), []byte(
		"tool:\n  artifact:\n    any:\n      url: file://"+resourceDir+"\n",
	), 0o600); err != nil {
		t.Fatalf("write resource specification: %v", err)
	}
	ctx := testResolverContext(t, resource.ResourceSpec{})

	// When
	layered, _, err := ResourceContext(ctx, Infrastructure, PresetRef{Path: presetDir})
	if err != nil {
		t.Fatalf("build preset resource context: %v", err)
	}
	path, err := resource.FromContext(layered).Resolve(layered, "tool")
	if err != nil {
		t.Fatalf("resolve preset resource: %v", err)
	}

	// Then
	if path != resourceDir {
		t.Fatalf("expected %q, got %q", resourceDir, path)
	}
	_, err = resource.FromContext(ctx).Resolve(ctx, "tool")
	if !errors.Is(err, resource.ErrUnknownMember) {
		t.Fatalf("expected preset resource to stay scoped, got %v", err)
	}
}

func TestResourceContextOverridesLauncherResourcesTemporarily(t *testing.T) {
	t.Parallel()

	// Given
	const baseContent = "launcher"
	const overrideContent = "preset"
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter, request *http.Request,
	) {
		if request.URL.Path == "/base" {
			_, _ = writer.Write([]byte(baseContent))
		} else {
			_, _ = writer.Write([]byte(overrideContent))
		}
	}))
	defer server.Close()
	presetDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(presetDir, resourceSpecFilename), []byte(
		fmt.Sprintf(
			"tool:\n  artifact:\n    any:\n      url: %s/override\n      sha256: %x\n",
			server.URL, sha256.Sum256([]byte(overrideContent)),
		),
	), 0o600); err != nil {
		t.Fatalf("write resource specification: %v", err)
	}
	ctx := testResolverContext(t, resource.ResourceSpec{
		"tool": {Artifact: map[string]resource.ArtifactSpec{
			"any": {
				URL:    server.URL + "/base",
				Sha256: fmt.Sprintf("%x", sha256.Sum256([]byte(baseContent))),
			},
		}},
	})

	// When
	layered, _, err := ResourceContext(ctx, Infrastructure, PresetRef{Path: presetDir})
	if err != nil {
		t.Fatalf("build preset resource context: %v", err)
	}
	overridden, err := resource.FromContext(layered).Resolve(layered, "tool")
	if err != nil {
		t.Fatalf("resolve overridden resource: %v", err)
	}
	base, err := resource.FromContext(ctx).Resolve(ctx, "tool")
	if err != nil {
		t.Fatalf("resolve base resource: %v", err)
	}

	// Then
	if overridden == base {
		t.Fatalf("override and launcher resource shared %q", overridden)
	}
	for path, want := range map[string]string{overridden: overrideContent, base: baseContent} {
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("read resource: %v", readErr)
		}
		if string(content) != want {
			t.Fatalf("resource %q contains %q, want %q", path, content, want)
		}
	}
}

func TestResourceContextKeepsPresetLayersIsolated(t *testing.T) {
	t.Parallel()

	// Given
	infrastructureDir := t.TempDir()
	installationDir := t.TempDir()
	wrongInfrastructureDir := t.TempDir()
	wrongInstallationDir := t.TempDir()
	launcherTool := t.TempDir()
	infrastructureTool := t.TempDir()
	installationTool := t.TempDir()
	infrastructureOnly := t.TempDir()
	if err := os.WriteFile(filepath.Join(infrastructureDir, resourceSpecFilename), []byte(
		"installation-presets/custom:\n"+
			"  artifact:\n    any:\n      url: file://"+wrongInstallationDir+"\n"+
			"tool:\n  artifact:\n    any:\n      url: file://"+infrastructureTool+"\n"+
			"infrastructure-only:\n  artifact:\n    any:\n"+
			"      url: file://"+infrastructureOnly+"\n",
	), 0o600); err != nil {
		t.Fatalf("write infrastructure resources: %v", err)
	}
	if err := os.WriteFile(filepath.Join(installationDir, resourceSpecFilename), []byte(
		"infrastructure-presets/custom:\n"+
			"  artifact:\n    any:\n      url: file://"+wrongInfrastructureDir+"\n"+
			"tool:\n  artifact:\n    any:\n      url: file://"+installationTool+"\n",
	), 0o600); err != nil {
		t.Fatalf("write installation resources: %v", err)
	}
	base := testResolverContext(t, resource.ResourceSpec{
		"infrastructure-presets/custom": memberDef(infrastructureDir),
		"installation-presets/custom":   memberDef(installationDir),
		"tool":                          memberDef(launcherTool),
	})

	// When
	infrastructureContext, resolvedInfrastructureDir, err := ResourceContext(
		base, Infrastructure, PresetRef{Name: "custom"},
	)
	if err != nil {
		t.Fatalf("add infrastructure resources: %v", err)
	}
	installationContext, resolvedInstallationDir, err := ResourceContext(
		base, Installation, PresetRef{Name: "custom"},
	)
	if err != nil {
		t.Fatalf("add installation resources: %v", err)
	}
	infrastructureResolvedTool, err := resource.FromContext(infrastructureContext).Resolve(
		infrastructureContext, "tool",
	)
	if err != nil {
		t.Fatalf("resolve infrastructure resource: %v", err)
	}
	installationResolvedTool, err := resource.FromContext(installationContext).Resolve(
		installationContext, "tool",
	)
	if err != nil {
		t.Fatalf("resolve installation resource: %v", err)
	}

	// Then
	if resolvedInfrastructureDir != infrastructureDir {
		t.Fatalf(
			"infrastructure directory = %q, want %q",
			resolvedInfrastructureDir,
			infrastructureDir,
		)
	}
	if resolvedInstallationDir != installationDir {
		t.Fatalf(
			"installation directory = %q, want %q",
			resolvedInstallationDir,
			installationDir,
		)
	}
	if infrastructureResolvedTool != infrastructureTool {
		t.Fatalf(
			"infrastructure tool = %q, want %q",
			infrastructureResolvedTool,
			infrastructureTool,
		)
	}
	if installationResolvedTool != installationTool {
		t.Fatalf(
			"installation tool = %q, want %q",
			installationResolvedTool,
			installationTool,
		)
	}
	_, err = resource.FromContext(installationContext).Resolve(
		installationContext, "infrastructure-only",
	)
	if !errors.Is(err, resource.ErrUnknownMember) {
		t.Fatalf("expected infrastructure resource to stay scoped, got %v", err)
	}
	launcherResolvedTool, err := resource.FromContext(base).Resolve(base, "tool")
	if err != nil {
		t.Fatalf("resolve launcher resource: %v", err)
	}
	if launcherResolvedTool != launcherTool {
		t.Fatalf("launcher tool = %q, want %q", launcherResolvedTool, launcherTool)
	}
}

//nolint:paralleltest // Working directory changes process-wide state.
func TestResourceContextResolvesRelativeLocationsFromPresetDirectory(t *testing.T) {
	// Given
	presetDir := t.TempDir()
	resourceDir := filepath.Join(presetDir, "resource")
	if err := os.Mkdir(resourceDir, 0o700); err != nil {
		t.Fatalf("create resource directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(presetDir, resourceSpecFilename), []byte(
		"tool:\n  artifact:\n    any:\n      url: resource\n",
	), 0o600); err != nil {
		t.Fatalf("write resource specification: %v", err)
	}
	ctx := testResolverContext(t, resource.ResourceSpec{})

	for range 2 {
		t.Chdir(t.TempDir())

		// When
		layered, _, err := ResourceContext(ctx, Infrastructure, PresetRef{Path: presetDir})
		if err != nil {
			t.Fatalf("build preset resource context: %v", err)
		}
		path, err := resource.FromContext(layered).Resolve(layered, "tool")
		// Then
		if err != nil {
			t.Fatalf("resolve preset resource: %v", err)
		}
		if path != resourceDir {
			t.Errorf("resolved path = %q, want %q", path, resourceDir)
		}
	}
}

func TestPresetDir_UnknownInstallationNameReturnsErrUnknownInstallation(t *testing.T) {
	t.Parallel()

	// Given
	ctx := testResolverContext(t, resource.ResourceSpec{})

	// When
	_, err := presetDir(ctx, Installation, "this-preset-does-not-exist")
	// Then
	if !errors.Is(err, ErrUnknownInstallation) {
		t.Fatalf("expected ErrUnknownInstallation, got %v", err)
	}
}

func TestListEmbeddedPresets_ReturnsNamesDeclaredUnderKind(t *testing.T) {
	t.Parallel()

	// Given
	ctx := testResolverContext(t, resource.ResourceSpec{
		infrastructurePresetsResource + "/aws":   memberDef(t.TempDir()),
		infrastructurePresetsResource + "/azure": memberDef(t.TempDir()),
	})

	// When
	names := ListEmbeddedPresets(ctx, Infrastructure)
	// Then
	if len(names) != 2 || names[0] != "aws" || names[1] != "azure" {
		t.Fatalf("expected [aws azure], got %v", names)
	}
}

func TestListEmbeddedPresets_EmptyForKindWithNoMembers(t *testing.T) {
	t.Parallel()

	// Given
	ctx := testResolverContext(t, resource.ResourceSpec{})

	// When
	names := ListEmbeddedPresets(ctx, Infrastructure)
	// Then
	if len(names) != 0 {
		t.Fatalf("expected no preset names, got %v", names)
	}
}

func TestWriteDir_NamedPresetCopiesResolvedDirectory(t *testing.T) {
	t.Parallel()

	// Given
	srcDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(srcDir, "aws"), 0o700); err != nil {
		t.Fatalf("failed to create fixture subdirectory: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(srcDir, "aws", "infrastructure.yaml"),
		[]byte("content"),
		0o600,
	); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}
	ctx := testResolverContext(t, resource.ResourceSpec{
		infrastructurePresetsResource + "/aws": memberDef(filepath.Join(srcDir, "aws")),
	})
	outDir := filepath.Join(t.TempDir(), "out")

	// When
	err := WriteDir(ctx, Infrastructure, PresetRef{Name: "aws"}, outDir)
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "infrastructure.yaml")); err != nil {
		t.Fatalf("expected copied file to exist, got %v", err)
	}
}

func TestWriteDir_UnknownNamedPresetReturnsErrUnknownInfrastructure(t *testing.T) {
	t.Parallel()

	// Given
	ctx := testResolverContext(t, resource.ResourceSpec{})

	// When
	err := WriteDir(ctx, Infrastructure, PresetRef{Name: "does-not-exist"}, t.TempDir())
	// Then
	if !errors.Is(err, ErrUnknownInfrastructure) {
		t.Fatalf("expected ErrUnknownInfrastructure, got %v", err)
	}
}

func TestWriteDir_PathPresetCopiesDirectoryDirectly(t *testing.T) {
	t.Parallel()

	// Given
	srcDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(srcDir, "installation.yaml"),
		[]byte("content"),
		0o600,
	); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}
	ctx := testResolverContext(t, resource.ResourceSpec{})
	outDir := filepath.Join(t.TempDir(), "out")

	// When
	err := WriteDir(ctx, Installation, PresetRef{Path: srcDir}, outDir)
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "installation.yaml")); err != nil {
		t.Fatalf("expected copied file to exist, got %v", err)
	}
}

func TestWriteDir_ResolutionFailurePropagatesInsteadOfReportingUnknownPreset(t *testing.T) {
	t.Parallel()

	// Given
	ctx := testResolverContext(t, resource.ResourceSpec{
		infrastructurePresetsResource + "/aws": memberDef(
			filepath.Join(t.TempDir(), "does-not-exist"),
		),
	})

	// When
	err := WriteDir(ctx, Infrastructure, PresetRef{Name: "aws"}, t.TempDir())
	// Then
	if err == nil {
		t.Fatal("expected an error, got none")
	}
	if errors.Is(err, ErrUnknownInfrastructure) {
		t.Fatalf("expected the underlying resolution error, got the unknown-preset sentinel: %v",
			err)
	}
}

func TestReadInfrastructureFile_RejectsPathEscapingThePresetDirectory(t *testing.T) {
	t.Parallel()

	// Given
	presetsRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(presetsRoot, "aws"), 0o700); err != nil {
		t.Fatalf("failed to create fixture subdirectory: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(presetsRoot, "secret.txt"), []byte("outside the preset"), 0o600,
	); err != nil {
		t.Fatalf("failed to write fixture file: %v", err)
	}
	ctx := testResolverContext(t, resource.ResourceSpec{
		infrastructurePresetsResource + "/aws": memberDef(filepath.Join(presetsRoot, "aws")),
	})

	// When
	_, err := ReadInfrastructureFile(ctx, "aws", "../secret.txt")
	// Then
	if err == nil {
		t.Fatal("expected an error for a relPath escaping the preset directory, got none")
	}
}

func TestWriteSharedDir_WritesSharedAssets(t *testing.T) {
	t.Parallel()

	ctx := resourcetest.NewContext(t)
	outDir := t.TempDir()

	if err := WriteSharedDir(ctx, outDir); err != nil {
		t.Fatalf("WriteSharedDir: %v", err)
	}

	for _, name := range []string{"eula.txt", "sample.sql"} {
		if _, err := os.Stat(filepath.Join(outDir, name)); err != nil {
			t.Fatalf("expected %s in the deployment directory: %v", name, err)
		}
	}
}

func memberDef(dir string) resource.ResourceDefinition {
	return resource.ResourceDefinition{
		Artifact: map[string]resource.ArtifactSpec{"any": {URL: dir}},
	}
}
