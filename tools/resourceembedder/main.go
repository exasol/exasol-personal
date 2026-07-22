// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

// Command resourceembedder generates the //go:embed wrapper file under
// assets/resources/generated/ for every resource marked embed: true in
// resources.yaml, for one target platform per invocation (default: the host
// platform, overridable via -goos/-goarch or the TARGET_GOOS/TARGET_GOARCH
// environment variables). All resources declared for a given platform are
// combined into a single generated file for that platform. It never imports
// assets/resources/generated itself, which is what guarantees it always
// performs a real, checksum-verified fetch rather than reusing whatever a
// previous run happened to embed — unless -skip-embed/SKIP_EMBED is set, in
// which case it always writes an empty placeholder instead, for callers that
// only need the package to compile and never look at the embedded bytes.
package main

import (
	"bytes"
	"context"
	_ "embed" // required by go:embed on resourceFileTemplateSource below
	"errors"
	"flag"
	"fmt"
	"go/format"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"text/template"

	"github.com/exasol/exasol-personal/assets/resources"
	"github.com/exasol/exasol-personal/internal/runtimeartifacts"
)

const (
	dirPerm             = 0o700
	filePerm            = 0o600
	generatedDirRelPath = "assets/resources/generated"
	generatedPkg        = "generated"
)

//go:embed resource_file.go.tmpl
var resourceFileTemplateSource string

var resourceFileTemplate = template.Must(template.New("resourceFile").Parse(resourceFileTemplateSource))

// repoRoot resolves the repository root from this source file's own location
// rather than the process's working directory: go generate invokes this tool
// with the working directory set to wherever the //go:generate directive
// lives (internal/runtimeartifacts/), not the repo root, so a bare relative
// path would resolve to the wrong place depending on how the tool was
// invoked (go generate vs. a Task step run from the repo root).
func repoRoot() (string, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("failed to resolve resourceembedder's own source path")
	}

	// This file lives at <repoRoot>/tools/resourceembedder/main.go.
	return filepath.Abs(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}

type config struct {
	goos      string
	goarch    string
	skipEmbed bool
}

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	cfg, err := parseFlags(args)
	if err != nil {
		return err
	}

	root, err := repoRoot()
	if err != nil {
		return err
	}
	outputDir := filepath.Join(root, generatedDirRelPath)

	spec, err := runtimeartifacts.ParseSpec(resources.ResourcesYAML)
	if err != nil {
		return err
	}

	cache, err := runtimeartifacts.NewDefaultCache()
	if err != nil {
		return err
	}

	g := &generator{
		manager: runtimeartifacts.NewResourceManagerWithCacheForPlatform(
			spec,
			cache,
			cfg.goos,
			cfg.goarch,
		),
		outputDir: outputDir,
		goos:      cfg.goos,
		goarch:    cfg.goarch,
		skipEmbed: cfg.skipEmbed,
	}

	return g.generatePlatform(ctx, spec)
}

func parseFlags(args []string) (config, error) {
	flags := flag.NewFlagSet("resourceembedder", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	// TARGET_GOOS/TARGET_GOARCH, not GOOS/GOARCH: the latter would also make
	// `go run`'s own toolchain cross-compile this tool instead of just
	// running it natively and picking a different target internally.
	cfg := config{goos: runtime.GOOS, goarch: runtime.GOARCH}
	if goos := strings.TrimSpace(os.Getenv("TARGET_GOOS")); goos != "" {
		cfg.goos = goos
	}
	if goarch := strings.TrimSpace(os.Getenv("TARGET_GOARCH")); goarch != "" {
		cfg.goarch = goarch
	}
	cfg.skipEmbed = strings.TrimSpace(os.Getenv("SKIP_EMBED")) == "true"
	flags.StringVar(&cfg.goos, "goos", cfg.goos, "Target GOOS")
	flags.StringVar(&cfg.goarch, "goarch", cfg.goarch, "Target GOARCH")
	flags.BoolVar(
		&cfg.skipEmbed,
		"skip-embed",
		cfg.skipEmbed,
		"Never fetch real artifact data; always write an empty placeholder (for lint/test builds that never look at the embedded bytes)",
	)
	if err := flags.Parse(args); err != nil {
		return config{}, err
	}

	return cfg, nil
}

type resourceEmbed struct {
	ResourceID string
	VarName    string
	DataFile   string
}

// platformFileData is the template input for one platform's generated file.
// Every embed: true resource that declares data for GOOS/GOARCH contributes
// one entry to Resources; every one that doesn't (or was skipped) is listed
// in Skipped purely for the generated file's own explanatory comment.
type platformFileData struct {
	GOOS, GOARCH string
	Package      string
	Resources    []resourceEmbed
	Skipped      []string
}

type generator struct {
	manager   *runtimeartifacts.Manager
	outputDir string
	goos      string
	goarch    string
	skipEmbed bool
}

// generatePlatform combines every embed: true resource in spec that declares
// data for g.goos/g.goarch into a single generated file for that platform.
func (g *generator) generatePlatform(ctx context.Context, spec runtimeartifacts.ResourceSpec) error {
	resourceIDs := make([]string, 0, len(spec))
	for resourceID, def := range spec {
		if def.Embed {
			resourceIDs = append(resourceIDs, resourceID)
		}
	}
	sort.Strings(resourceIDs) // deterministic output; map iteration order isn't.

	data := platformFileData{GOOS: g.goos, GOARCH: g.goarch, Package: generatedPkg}
	for _, resourceID := range resourceIDs {
		embed, err := g.resolveResourceEmbed(ctx, resourceID, spec[resourceID])
		if err != nil {
			return fmt.Errorf(
				"failed to generate embedded resource %q for %s/%s: %w", resourceID, g.goos, g.goarch, err,
			)
		}
		if embed != nil {
			data.Resources = append(data.Resources, *embed)
		} else {
			data.Skipped = append(data.Skipped, resourceID)
		}
	}

	return writePlatformFile(g.outputDir, data)
}

// resolveResourceEmbed fetches and stages one resource's raw artifact bytes
// for g.goos/g.goarch, returning nil (not an error) when there's nothing to
// embed: g.skipEmbed is set, or the resource declares no artifact for this
// platform.
func (g *generator) resolveResourceEmbed(
	ctx context.Context,
	resourceID string,
	def runtimeartifacts.ResourceDefinition,
) (*resourceEmbed, error) {
	if g.skipEmbed {
		// Lint/test builds never look at the embedded bytes, so there's
		// nothing for them to gain from a real fetch even once, cached or
		// not: skip it unconditionally and just satisfy the build constraint.
		return nil, nil
	}

	if _, err := def.Resolve(g.goos, g.goarch); err != nil {
		return nil, nil
	}

	// The real resources.yaml entry has extract: true and embed: true for this
	// resource, which would make Get return the post-extraction resolved path
	// and, once assets/resources/generated is linked into a process, prefer
	// already-embedded bytes over a real fetch. Forcing both false here
	// guarantees a real, checksum-verified network fetch of the raw artifact
	// every time this tool runs, regardless of what resources.yaml declares or
	// what the current process happens to have linked.
	rawDef := def
	rawDef.Extract = false
	rawDef.Embed = false

	rawPath, err := g.manager.Get(ctx, rawDef, resourceID)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(rawPath)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(g.outputDir, dirPerm); err != nil {
		return nil, err
	}
	dataFile := dataFileName(resourceID, g.goos, g.goarch)
	if err := os.WriteFile(filepath.Join(g.outputDir, dataFile), data, filePerm); err != nil {
		return nil, err
	}
	fmt.Fprintf(os.Stdout, "Staged embedded resource %s (%s/%s): %s\n", resourceID, g.goos, g.goarch, dataFile)

	return &resourceEmbed{
		ResourceID: resourceID,
		VarName:    goIdentifier(resourceID) + "Data",
		DataFile:   dataFile,
	}, nil
}

// writePlatformFile gofmt-formats the rendered template output before
// writing it, so the template itself doesn't need exact whitespace.
func writePlatformFile(outputDir string, data platformFileData) error {
	var buf bytes.Buffer
	if err := resourceFileTemplate.Execute(&buf, data); err != nil {
		return err
	}

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return fmt.Errorf("generated file for %s/%s is not valid Go: %w", data.GOOS, data.GOARCH, err)
	}

	if err := os.MkdirAll(outputDir, dirPerm); err != nil {
		return err
	}
	path := filepath.Join(outputDir, platformFileName(data.GOOS, data.GOARCH))
	if err := os.WriteFile(path, formatted, filePerm); err != nil {
		return err
	}

	if len(data.Resources) == 0 {
		fmt.Fprintf(os.Stdout, "Generated placeholder for %s/%s (no embedded resources): %s\n", data.GOOS, data.GOARCH, path)
	} else {
		fmt.Fprintf(
			os.Stdout,
			"Generated %d embedded resource(s) for %s/%s: %s\n",
			len(data.Resources), data.GOOS, data.GOARCH, path,
		)
	}

	return nil
}

func platformFileName(goos, goarch string) string {
	return "resources_" + goos + "_" + goarch + ".go"
}

func dataFileName(resourceID, goos, goarch string) string {
	return strings.ReplaceAll(resourceID, "-", "_") + "_" + goos + "_" + goarch + ".bin"
}

// goIdentifier turns a kebab-case resource ID into a camelCase Go identifier.
// Every resource embedded for a platform shares one generated file, so this
// must be unique per resource within that file.
func goIdentifier(resourceID string) string {
	parts := strings.FieldsFunc(resourceID, func(r rune) bool {
		return r == '-' || r == '_'
	})

	var identifier strings.Builder
	for i, part := range parts {
		if part == "" {
			continue
		}
		if i == 0 {
			identifier.WriteString(strings.ToLower(part))

			continue
		}
		identifier.WriteString(strings.ToUpper(part[:1]))
		identifier.WriteString(strings.ToLower(part[1:]))
	}

	return identifier.String()
}
