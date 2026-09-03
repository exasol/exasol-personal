// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/exasol/exasol-personal/assets/resourcedata"
	"github.com/exasol/exasol-personal/internal/resource"
	"go.yaml.in/yaml/v3"
)

const (
	dirPerm          = 0o700
	filePerm         = 0o600
	embeddedDirRel   = "assets/resourcedata/embedded/data"
	resolvedSpecName = "resolved.yaml"
	blobsDirName     = "blobs"
	gitignoreName    = ".gitignore"
	tarGzExtension   = ".tar.gz"
)

// go generate runs tools from the directive's directory, not the repository root.
func repoRoot() (string, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("failed to resolve resourceembedder's own source path")
	}

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
	if !(resource.FileSource{}).Handles(artifact.Locator()) {
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

	spec, err := resource.ParseSpec(resourcedata.ResourcesYAML)
	if err != nil {
		return err
	}
	spec = resolveRelativeLocalArtifacts(root, spec)

	cache, err := resource.NewDefaultCache()
	if err != nil {
		return err
	}

	resolver, err := resource.New(resource.Options{
		Definitions: spec,
		Cache:       cache,
		Platform:    resource.Platform{GOOS: cfg.goos, GOARCH: cfg.goarch},
	})
	if err != nil {
		return err
	}

	g := &generator{
		resolver:    resolver,
		platformDir: filepath.Join(root, embeddedDirRel, cfg.goos+"_"+cfg.goarch),
		goos:        cfg.goos,
		goarch:      cfg.goarch,
		skipEmbed:   cfg.skipEmbed,
	}

	return g.generate(ctx, spec)
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
		"Embed nothing beyond `embed: always` resources, pointing the rest upstream",
	)
	if err := flags.Parse(args); err != nil {
		return config{}, err
	}

	return cfg, nil
}

type generator struct {
	resolver    *resource.Resolver
	platformDir string
	goos        string
	goarch      string
	skipEmbed   bool
	written     map[string]bool
}

func (g *generator) generate(ctx context.Context, spec resource.ResourceSpec) error {
	g.written = map[string]bool{gitignoreName: true}

	resourceIDs := make([]string, 0, len(spec))
	for resourceID := range spec {
		resourceIDs = append(resourceIDs, resourceID)
	}
	sort.Strings(resourceIDs) // deterministic output; map iteration order isn't.

	resolved := resource.ResourceSpec{}
	for _, resourceID := range resourceIDs {
		entries, err := g.resolveResource(ctx, resourceID, spec[resourceID])
		if err != nil {
			return fmt.Errorf(
				"failed to generate resource %q for %s/%s: %w", resourceID, g.goos, g.goarch, err,
			)
		}
		for id, def := range entries {
			resolved[id] = def
		}
	}

	if err := g.writeResolvedSpec(resolved); err != nil {
		return err
	}

	return g.prune()
}

func (g *generator) resolveResource(
	ctx context.Context,
	resourceID string,
	def resource.ResourceDefinition,
) (resource.ResourceSpec, error) {
	artifact, err := def.Resolve(g.goos, g.goarch)
	if err != nil {
		return nil, nil //nolint:nilnil
	}

	if def.Glob != "" {
		return g.expandGlob(ctx, resourceID, def, artifact)
	}

	if !g.shouldEmbed(def) {
		if err := rejectUnembeddedLocalSource(resourceID, artifact); err != nil {
			return nil, err
		}

		return resource.ResourceSpec{resourceID: upstreamEntry(def, artifact)}, nil
	}

	data, err := g.rawResourceBytes(ctx, resourceID, def)
	if err != nil {
		return nil, err
	}
	blob, err := g.writeBlob(resourceID, blobExtension(artifact), data)
	if err != nil {
		return nil, err
	}

	return resource.ResourceSpec{resourceID: embeddedEntry(def.Extract, blob, artifact.Subpath)}, nil
}

// Groups are build-only; only their members enter the resolved specification.
func (g *generator) expandGlob(
	ctx context.Context,
	resourceID string,
	def resource.ResourceDefinition,
	artifact resource.ArtifactSpec,
) (resource.ResourceSpec, error) {
	root, err := g.resolve(ctx, resourceID, rawDefFor(def))
	if err != nil {
		return nil, err
	}

	matches, err := globMatches(root, def.Glob)
	if err != nil {
		return nil, err
	}

	embed := g.shouldEmbed(def)
	names := make([]string, 0, len(matches))
	for name := range matches {
		names = append(names, name)
	}
	sort.Strings(names)

	resolved := resource.ResourceSpec{}
	for _, name := range names {
		memberID := resourceID + "/" + name
		if !embed {
			member := artifact
			matchPath, err := filepath.Rel(root, matches[name])
			if err != nil {
				return nil, err
			}
			member.Subpath = path.Join(artifact.Subpath, filepath.ToSlash(matchPath))
			if err := rejectUnembeddedLocalSource(memberID, member); err != nil {
				return nil, err
			}
			resolved[memberID] = upstreamEntry(def, member)

			continue
		}

		data, err := readResourceBytes(matches[name])
		if err != nil {
			return nil, err
		}
		blob, err := g.writeBlob(memberID, memberExtension(matches[name]), data)
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(matches[name])
		if err != nil {
			return nil, err
		}
		resolved[memberID] = embeddedEntry(info.IsDir(), blob, "")
	}

	return resolved, nil
}

func (g *generator) shouldEmbed(def resource.ResourceDefinition) bool {
	switch def.Embed {
	case resource.EmbedAlways:
		return true
	case resource.EmbedDefault:
		return !g.skipEmbed
	case resource.EmbedNever:
		return false
	default:
		return false
	}
}

// Build input must not depend on output left by an earlier run.
func rawDefFor(def resource.ResourceDefinition) resource.ResourceDefinition {
	rawDef := def
	rawDef.Embed = resource.EmbedNever
	rawDef.Glob = ""

	return rawDef
}

func (g *generator) rawResourceBytes(
	ctx context.Context,
	resourceID string,
	def resource.ResourceDefinition,
) ([]byte, error) {
	rawDef := rawDefFor(def)
	rawDef.Extract = false
	for platform, artifact := range rawDef.Artifact {
		artifact.Subpath = ""
		rawDef.Artifact[platform] = artifact
	}

	rawPath, err := g.resolve(ctx, resourceID, rawDef)
	if err != nil {
		return nil, err
	}

	return readResourceBytes(rawPath)
}

func (g *generator) resolve(
	ctx context.Context,
	resourceID string,
	def resource.ResourceDefinition,
) (string, error) {
	artifact, err := def.Resolve(g.goos, g.goarch)
	if err != nil {
		return "", fmt.Errorf("resolve resource %q: %w", resourceID, err)
	}

	return g.resolver.ResolveDescriptor(ctx, resource.Descriptor{
		Locator: artifact.Locator(), Sha256: artifact.Sha256,
		Extract: def.Extract, Subpath: artifact.Subpath,
		DownloadPath: artifact.DownloadPath,
	})
}

// Non-embedded local paths would be invalid outside this checkout.
func rejectUnembeddedLocalSource(resourceID string, artifact resource.ArtifactSpec) error {
	if !(resource.FileSource{}).Handles(artifact.Locator()) {
		return nil
	}

	return fmt.Errorf(
		"resource %q declares the local source %q but is not embedded,"+
			" so a built launcher could not reach it",
		resourceID, artifact.URL,
	)
}

func upstreamEntry(
	def resource.ResourceDefinition,
	artifact resource.ArtifactSpec,
) resource.ResourceDefinition {
	return resource.ResourceDefinition{
		Extract:  def.Extract,
		Artifact: map[string]resource.ArtifactSpec{"any": artifact},
	}
}

func embeddedEntry(extract bool, blob blobRef, subpath string) resource.ResourceDefinition {
	return resource.ResourceDefinition{
		Extract: extract,
		Artifact: map[string]resource.ArtifactSpec{
			"any": {
				URL:          resource.EmbeddedURLScheme + blob.path,
				Sha256:       blob.sha256,
				DownloadPath: path.Base(blob.path),
				Subpath:      subpath,
			},
		},
	}
}

type blobRef struct {
	path   string
	sha256 string
}

func (g *generator) writeBlob(resourceID, extension string, data []byte) (blobRef, error) {
	relPath := path.Join(blobsDirName, resourceID+extension)
	target := filepath.Join(g.platformDir, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(target), dirPerm); err != nil {
		return blobRef{}, err
	}
	if err := os.WriteFile(target, data, filePerm); err != nil {
		return blobRef{}, err
	}
	g.written[relPath] = true

	sum := sha256.Sum256(data)
	fmt.Fprintf(os.Stdout, "Staged %s (%s/%s): %s\n", resourceID, g.goos, g.goarch, relPath)

	return blobRef{path: relPath, sha256: hex.EncodeToString(sum[:])}, nil
}

func (g *generator) writeResolvedSpec(resolved resource.ResourceSpec) error {
	data, err := yaml.Marshal(resolved)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(g.platformDir, dirPerm); err != nil {
		return err
	}
	if err := os.WriteFile(
		filepath.Join(g.platformDir, resolvedSpecName), data, filePerm,
	); err != nil {
		return err
	}
	g.written[resolvedSpecName] = true

	fmt.Fprintf(
		os.Stdout, "Resolved %d resource(s) for %s/%s\n", len(resolved), g.goos, g.goarch,
	)

	return nil
}

// Stale files would ship because wrappers embed the whole platform directory.
func (g *generator) prune() error {
	var stale []string
	err := filepath.WalkDir(g.platformDir, func(pathValue string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		rel, relErr := filepath.Rel(g.platformDir, pathValue)
		if relErr != nil {
			return relErr
		}
		if !g.written[filepath.ToSlash(rel)] {
			stale = append(stale, pathValue)
		}

		return nil
	})
	if err != nil {
		return err
	}

	for _, pathValue := range stale {
		if err := os.Remove(pathValue); err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "Pruned %s\n", pathValue)
	}

	return removeEmptyDirs(g.platformDir)
}

func removeEmptyDirs(root string) error {
	var dirs []string
	err := filepath.WalkDir(root, func(pathValue string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && pathValue != root {
			dirs = append(dirs, pathValue)
		}

		return nil
	})
	if err != nil {
		return err
	}

	sort.Sort(sort.Reverse(sort.StringSlice(dirs)))
	for _, dir := range dirs {
		_ = os.Remove(dir)
	}

	return nil
}

// Repository metadata is never a resource, even when a top-level glob matches it.
func globMatches(root, pattern string) (map[string]string, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return nil, errors.New("glob pattern must not be empty")
	}

	globbed, err := filepath.Glob(filepath.Join(root, pattern))
	if err != nil {
		return nil, err
	}

	matches := make(map[string]string, len(globbed))
	for _, match := range globbed {
		base := filepath.Base(match)
		if base == ".git" {
			continue
		}
		if existing, ok := matches[base]; ok {
			return nil, fmt.Errorf(
				"pattern %q matches %q and %q, which share the member name %q",
				pattern, existing, match, base,
			)
		}
		matches[base] = match
	}

	return matches, nil
}

func memberExtension(matchPath string) string {
	info, err := os.Stat(matchPath)
	if err == nil && info.IsDir() {
		return tarGzExtension
	}

	return archiveExtension(filepath.Base(matchPath))
}

// Extractor selection depends on the generated blob's suffix.
func blobExtension(artifact resource.ArtifactSpec) string {
	if name := strings.TrimSpace(artifact.DownloadPath); name != "" {
		return archiveExtension(name)
	}
	if ext := archiveExtension(path.Base(artifact.Locator().URL)); ext != "" {
		return ext
	}

	return tarGzExtension
}

func archiveExtension(name string) string {
	for _, suffix := range []string{tarGzExtension, ".tgz", ".zip", ".tar"} {
		if strings.HasSuffix(name, suffix) {
			return suffix
		}
	}

	return filepath.Ext(name)
}

func readResourceBytes(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return tarGzDirectory(path)
	}

	return os.ReadFile(path)
}

// Entry timestamps are zeroed so unchanged content keeps the same identity.
// The scoped root prevents a symlink swap from redirecting a read outside it.
func tarGzDirectory(root string) ([]byte, error) {
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

// Readlink preserves links without following targets outside the scoped root.
func writeSymlinkEntry(
	tarWriter *tar.Writer, rootDir *os.Root, relPath string, info fs.FileInfo,
) error {
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
