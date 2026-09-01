// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package resource

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/exasol/exasol-personal/internal/util"
)

const (
	dirPerm        = 0o700
	filePerm       = 0o600
	extractRelPath = "unpack"
)

var ErrUnknownMember = errors.New("unknown member")

type Platform struct {
	GOOS   string
	GOARCH string
}

type Resolver struct {
	spec       *Specification
	cache      *Cache
	sources    []Source
	extractors []Extractor
}

type Probe struct {
	// Empty forces a transfer on every resolution.
	Identity string
	// A non-empty path bypasses artifact storage but can still be extracted.
	Local string
}

type Source interface {
	Handles(loc Locator) bool
	Probe(ctx context.Context, loc Locator) (Probe, error)
	Fetch(ctx context.Context, loc Locator, dstPath string) error
}

type Extractor interface {
	CanExtract(filename string) bool
	Extract(srcPath, dstPath string) error
}

func defaultSources(blobs fs.FS) []Source {
	return []Source{
		&GitSource{},
		&FileSource{},
		&HttpSource{},
		&DockerSource{},
		&EmbeddedSource{blobs: blobs},
	}
}

type Options struct {
	Spec []byte
	// Definitions takes precedence over Spec.
	Definitions ResourceSpec
	Blobs       fs.FS
	CacheRoot   string
	Cache       *Cache
	Platform    Platform
}

func New(opts Options) (*Resolver, error) {
	spec := opts.Definitions
	if spec == nil {
		var err error
		spec, err = ParseSpec(opts.Spec)
		if err != nil {
			return nil, err
		}
	}

	cache := opts.Cache
	if cache == nil {
		cache = opts.cache()
	}
	if cache == nil {
		defaultCache, err := NewDefaultCache()
		if err != nil {
			return nil, err
		}
		cache = defaultCache
	}

	platform := opts.Platform
	if platform.GOOS == "" {
		platform = Platform{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH}
	}

	return &Resolver{
		spec:       NewSpecification(spec, platform),
		cache:      cache,
		sources:    defaultSources(opts.Blobs),
		extractors: []Extractor{&TarGzExtractor{}, &ZipExtractor{}},
	}, nil
}

func (o Options) cache() *Cache {
	if strings.TrimSpace(o.CacheRoot) == "" {
		return nil
	}

	return NewCache(o.CacheRoot, filepath.Join(o.CacheRoot, cacheConfigFileName))
}

func (r *Resolver) Resolve(ctx context.Context, resourceID string) (string, error) {
	descriptor, err := r.spec.Lookup(resourceID)
	if err != nil {
		return "", err
	}

	return r.resolveArtifact(ctx, descriptor, resourceID)
}

func (r *Resolver) ResolveDescriptor(ctx context.Context, descriptor Descriptor) (string, error) {
	return r.resolveArtifact(ctx, descriptor, descriptor.Locator.String())
}

func Copy(path, destDir string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	if info.IsDir() {
		return util.CopyDir(path, destDir)
	}

	if err := os.MkdirAll(destDir, dirPerm); err != nil {
		return err
	}

	return copyFileInto(path, destDir)
}

func (r *Resolver) List(prefix string) []string {
	return r.spec.List(prefix)
}

func (r *Resolver) Layer(definitions ResourceSpec) *Resolver {
	return &Resolver{
		spec: &Specification{
			definitions: definitions,
			platform:    r.spec.platform,
			parent:      r.spec,
		},
		cache:      r.cache,
		sources:    r.sources,
		extractors: r.extractors,
	}
}

func (r *Resolver) Cache() *Cache {
	return r.cache
}

func copyFileInto(srcPath, destDir string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.Create(filepath.Join(destDir, filepath.Base(srcPath)))
	if err != nil {
		return err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return err
	}

	return dst.Close()
}

func contentIdentity(declaredSha256, probed string) string {
	if normalized := normalizeSha256(declaredSha256); normalized != "" {
		return "sha256:" + normalized
	}

	return probed
}

func (r *Resolver) sourceFor(loc Locator) (Source, error) {
	for _, source := range r.sources {
		if source.Handles(loc) {
			return source, nil
		}
	}

	return nil, fmt.Errorf("unsupported resource scheme in %q", loc.URL)
}

func (r *Resolver) probeSource(ctx context.Context, loc Locator) (Probe, error) {
	source, err := r.sourceFor(loc)
	if err != nil {
		return Probe{}, err
	}

	return source.Probe(ctx, loc)
}

func normalizeSha256(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func urlBasename(rawURL string) string {
	rawPath := rawURL
	if after, ok := strings.CutPrefix(rawPath, FileURLScheme); ok {
		rawPath = after
	} else if idx := strings.Index(rawPath, "://"); idx >= 0 {
		rawPath = rawPath[idx+3:]
	}
	base := filepath.Base(rawPath)
	if base == "." || base == "/" {
		return ""
	}

	return base
}
