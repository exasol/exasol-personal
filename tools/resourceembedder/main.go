// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

// Command resourceembedder generates the //go:embed wrapper file under
// assets/resourcedata/generated/ for every resource marked embed: true in
// resources.yaml, for one target platform per invocation (default: the host
// platform, overridable via -goos/-goarch or the TARGET_GOOS/TARGET_GOARCH
// environment variables). All resources declared for a given platform are
// combined into a single generated file for that platform. It never imports
// assets/resourcedata/generated itself, which is what guarantees it always
// performs a real, checksum-verified fetch rather than reusing whatever a
// previous run happened to embed — unless -skip-embed/SKIP_EMBED is set, in
// which case it always writes an empty placeholder instead, for callers that
// only need the package to compile and never look at the embedded bytes.
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	_ "embed" // required by go:embed on resourceFileTemplateSource below
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"go/format"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/exasol/exasol-personal/assets/resourcedata"
	"github.com/exasol/exasol-personal/internal/resource"
)

const (
	dirPerm             = 0o700
	filePerm            = 0o600
	generatedDirRelPath = "assets/resourcedata/generated"
	generatedPkg        = "generated"
)

//go:embed resource_file.go.tmpl
var resourceFileTemplateSource string

var resourceFileTemplate = template.Must(template.New("resourceFile").Parse(resourceFileTemplateSource))

// repoRoot resolves the repository root from this source file's own location
// rather than the process's working directory: go generate invokes this tool
// with the working directory set to wherever the //go:generate directive
// lives (internal/resource/), not the repo root, so a bare relative
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

// A resource's location must not depend on the invoking process's working
// directory, since go generate and a Task step run from the repo root can
// differ.
func resolveRelativeLocalArtifacts(
	root string,
	spec resource.ResourceSpec,
) resource.ResourceSpec {
	resolved := make(resource.ResourceSpec, len(spec))
	for resourceID, def := range spec {
		resolvedArtifacts := make(map[string]resource.ArtifactSpec, len(def.Artifact))
		for key, artifact := range def.Artifact {
			resolvedArtifacts[key] = resolveLocalArtifactURL(root, artifact)
		}
		def.Artifact = resolvedArtifacts
		resolved[resourceID] = def
	}

	return resolved
}

func resolveLocalArtifactURL(
	root string,
	artifact resource.ArtifactSpec,
) resource.ArtifactSpec {
	if !(resource.FileSource{}).CanFetch(artifact.URL) {
		return artifact
	}

	rawPath := strings.TrimPrefix(artifact.URL, resource.FileURLScheme)
	if filepath.IsAbs(rawPath) {
		return artifact
	}

	artifact.URL = resource.FileURLScheme + filepath.Join(root, rawPath)

	return artifact
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

	spec, err := resource.ParseSpec(resourcedata.ResourcesYAML)
	if err != nil {
		return err
	}

	spec = resolveRelativeLocalArtifacts(root, spec)

	cache, err := resource.NewDefaultCache()
	if err != nil {
		return err
	}

	g := &generator{
		manager: resource.NewResourceManagerWithCacheForPlatform(
			spec, cache, cfg.goos, cfg.goarch,
		),
		outputDir: outputDir,
		goos:      cfg.goos,
		goarch:    cfg.goarch,
		skipEmbed: cfg.skipEmbed,
	}

	return g.generatePlatform(ctx, spec)
}

// This tool never imports assets/resourcedata/generated, so it has no embedded
// data to fall back to; a declared Embed would make Get look for exactly
// that, and fail. resource_path is cleared too, since Get would otherwise
// treat it as a literal subdirectory name rather than the pattern it
// actually is.
func rawDefFor(def resource.ResourceDefinition) resource.ResourceDefinition {
	rawDef := def
	rawDef.Embed = resource.EmbedNever
	rawDef.Artifact = make(map[string]resource.ArtifactSpec, len(def.Artifact))
	for platform, artifact := range def.Artifact {
		artifact.ResourcePath = ""
		rawDef.Artifact[platform] = artifact
	}

	return rawDef
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
	// Members holds the name of every entry a Glob:true resource matched at
	// generation time, empty for a resource that isn't a glob template.
	Members []string
	// Hash is a content hash of this resource's embedded bytes, computed
	// once here at generation time so the running binary never has to pay
	// to hash its own embedded data on every startup.
	Hash string
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
	manager   *resource.Manager
	outputDir string
	goos      string
	goarch    string
	skipEmbed bool
}

// generatePlatform combines every embed: true resource in spec that declares
// data for g.goos/g.goarch into a single generated file for that platform.
func (g *generator) generatePlatform(ctx context.Context, spec resource.ResourceSpec) error {
	resourceIDs := make([]string, 0, len(spec))
	for resourceID, def := range spec {
		if def.Embed != resource.EmbedNever {
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

// Returning nil, not an error, means there's nothing to embed: the resource
// declares no artifact for this platform, or g.skipEmbed is set and the
// artifact requires a real, potentially slow or networked fetch.
func (g *generator) resolveResourceEmbed(
	ctx context.Context,
	resourceID string,
	def resource.ResourceDefinition,
) (*resourceEmbed, error) {
	artifact, err := def.Resolve(g.goos, g.goarch)
	if err != nil {
		return nil, nil
	}

	if g.skipEmbed && def.Embed != resource.EmbedAlways {
		// embed: always is the one exemption: it must still resolve to real
		// data regardless of build speed concerns.
		return nil, nil
	}

	if def.Glob {
		return g.resolveGlobEmbed(ctx, resourceID, def, artifact)
	}

	return g.resolveRawEmbed(ctx, resourceID, def)
}

// This always fetches without extracting, regardless of what resources.yaml
// declares: a running binary extracts the embedded bytes itself later if
// the real entry declares extract: true.
func (g *generator) resolveRawEmbed(
	ctx context.Context, resourceID string, def resource.ResourceDefinition,
) (*resourceEmbed, error) {
	rawDef := rawDefFor(def)
	rawDef.Extract = false

	rawPath, err := g.manager.Get(ctx, rawDef, resourceID)
	if err != nil {
		return nil, err
	}
	data, err := readResourceBytes(rawPath)
	if err != nil {
		return nil, err
	}

	return g.stageEmbed(resourceID, data, nil)
}

// A real directory is needed to glob within, so this fetches and extracts
// resourceID exactly as declared. The embedded archive holds only the
// matched entries, never the group's full content.
func (g *generator) resolveGlobEmbed(
	ctx context.Context,
	resourceID string,
	def resource.ResourceDefinition,
	artifact resource.ArtifactSpec,
) (*resourceEmbed, error) {
	root, err := g.manager.Get(ctx, rawDefFor(def), resourceID)
	if err != nil {
		return nil, err
	}

	matches, err := resource.GlobMatches(root, artifact.ResourcePath)
	if err != nil {
		return nil, err
	}

	members := make([]string, 0, len(matches))
	include := make(map[string]bool, len(matches))
	for name, matchPath := range matches {
		members = append(members, name)
		relPath, err := filepath.Rel(root, matchPath)
		if err != nil {
			return nil, err
		}
		include[filepath.ToSlash(relPath)] = true
	}
	sort.Strings(members)

	data, err := tarGzDirectory(root, include)
	if err != nil {
		return nil, err
	}

	return g.stageEmbed(resourceID, data, members)
}

func (g *generator) stageEmbed(resourceID string, data []byte, members []string) (*resourceEmbed, error) {
	if err := os.MkdirAll(g.outputDir, dirPerm); err != nil {
		return nil, err
	}
	dataFile := dataFileName(resourceID, g.goos, g.goarch)
	if err := os.WriteFile(filepath.Join(g.outputDir, dataFile), data, filePerm); err != nil {
		return nil, err
	}
	fmt.Fprintf(os.Stdout, "Staged embedded resource %s (%s/%s): %s\n", resourceID, g.goos, g.goarch, dataFile)

	sum := sha256.Sum256(data)

	return &resourceEmbed{
		ResourceID: resourceID,
		VarName:    goIdentifier(resourceID) + "Data",
		DataFile:   dataFile,
		Members:    members,
		Hash:       hex.EncodeToString(sum[:]),
	}, nil
}

// A directory is archived into a tar.gz in memory first, since embedding
// stores a flat byte blob; a file's bytes are used as-is.
func readResourceBytes(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return tarGzDirectory(path, nil)
	}

	return os.ReadFile(path)
}

// tarGzDirectory archives root's contents into a tar.gz byte stream. If
// include is non-nil, it names the archive's matched entries as slash-
// separated paths relative to root; each entry, its ancestor directories
// (needed to reach it), and everything beneath it are archived — this is how
// a Glob:true resource embeds only its matches, not root's full content,
// even for a pattern nested below root's own top level. Entry timestamps are
// zeroed so the output is byte-for-byte deterministic for identical
// directory content, regardless of the source files' own mtimes. Traversal
// and file reads go through an os.Root scoped to root, so a symlink swapped
// in after WalkDir stats an entry can't redirect the eventual open outside
// root.
func tarGzDirectory(root string, include map[string]bool) ([]byte, error) {
	rootDir, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rootDir.Close() }()

	var buf bytes.Buffer
	gzipWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzipWriter)

	err = fs.WalkDir(rootDir.FS(), ".", func(relPath string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if relPath == "." {
			return nil
		}
		if include != nil && !pathIncluded(include, relPath) {
			if entry.IsDir() {
				return fs.SkipDir
			}

			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}

		if info.Mode()&fs.ModeSymlink != 0 {
			return writeSymlinkEntry(tarWriter, rootDir, relPath, info)
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(relPath)
		if entry.IsDir() {
			header.Name += "/"
		}
		header.ModTime = time.Time{}
		header.AccessTime = time.Time{}
		header.ChangeTime = time.Time{}

		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}

		file, err := rootDir.Open(relPath)
		if err != nil {
			return err
		}
		defer func() { _ = file.Close() }()

		_, err = io.Copy(tarWriter, file)

		return err
	})
	if err != nil {
		return nil, err
	}

	if err := tarWriter.Close(); err != nil {
		return nil, err
	}
	if err := gzipWriter.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// pathIncluded reports whether relPath belongs in the archive: it is itself
// a matched entry, an ancestor directory of one, or nested beneath one.
func pathIncluded(include map[string]bool, relPath string) bool {
	if include[relPath] {
		return true
	}
	for matched := range include {
		if strings.HasPrefix(matched, relPath+"/") || strings.HasPrefix(relPath, matched+"/") {
			return true
		}
	}

	return false
}

// writeSymlinkEntry archives a symlink as a symlink, not as a copy of
// whatever it currently resolves to: rootDir.Open would follow it (and
// reject an escaping target), but a tar symlink header only needs the raw
// link text, which Readlink returns without following anything.
func writeSymlinkEntry(tarWriter *tar.Writer, rootDir *os.Root, relPath string, info fs.FileInfo) error {
	target, err := rootDir.Readlink(relPath)
	if err != nil {
		return err
	}
	header, err := tar.FileInfoHeader(info, target)
	if err != nil {
		return err
	}
	header.Name = filepath.ToSlash(relPath)
	header.ModTime = time.Time{}
	header.AccessTime = time.Time{}
	header.ChangeTime = time.Time{}

	return tarWriter.WriteHeader(header)
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
	flat := strings.ReplaceAll(resourceID, "-", "_")

	return flat + "_" + goos + "_" + goarch + ".bin"
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
