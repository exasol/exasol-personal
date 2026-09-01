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
	"slices"
	"strings"

	"github.com/exasol/exasol-personal/internal/util"
)

const (
	dirPerm        = 0o700
	filePerm       = 0o600
	extractRelPath = "unpack"
)

// ErrUnknownMember distinguishes an unknown ID from a failure to resolve a
// declared resource.
var ErrUnknownMember = errors.New("unknown member")

type Platform struct {
	GOOS   string
	GOARCH string
}

type Resolver struct {
	spec       ResourceSpec
	cache      *Cache
	platform   Platform
	sources    []Source
	extractors []Extractor
}

// Probe is what a source can determine about a locator without transferring
// its content.
type Probe struct {
	// Identity changes exactly when the content changes. It is opaque to the
	// resolver, which only keys the cache on it; each source formats its own.
	// Empty means the source cannot tell, and the resource is refreshed on
	// every request.
	Identity string
	// Local names a path that already holds the artifact. When set, Fetch is
	// never called and the cache stores no copy of the artifact itself, though
	// an extraction of it is still cached.
	Local string
}

type Source interface {
	Handles(loc Locator) bool
	Probe(ctx context.Context, loc Locator) (Probe, error)
	// Fetch places the content at loc into dstPath.
	Fetch(ctx context.Context, loc Locator, dstPath string) error
}

type Extractor interface {
	CanExtract(filename string) bool
	Extract(srcPath, dstPath string) error
}

// defaultSources orders the sources a resolver dispatches through. Blobs may
// be nil, in which case nothing resolves from embedded data.
func defaultSources(blobs fs.FS) []Source {
	return []Source{
		&GitSource{},
		&FileSource{},
		&HttpSource{},
		&DockerSource{},
		&EmbeddedSource{blobs: blobs},
	}
}

// Options describes the resolver a caller wants.
type Options struct {
	// Spec is the resolved resource specification, as YAML.
	Spec []byte
	// Definitions avoids serializing a specification that is already available
	// in memory.
	Definitions ResourceSpec
	// Blobs holds data compiled into the binary, addressed by the paths Spec
	// names. Nil means nothing is embedded and every resource resolves from
	// its upstream source.
	Blobs fs.FS
	// CacheRoot overrides where resources are stored. Empty uses the per-user
	// default.
	CacheRoot string
	// Cache overrides the cache constructed from CacheRoot.
	Cache *Cache
	// Platform overrides the host platform, for a build targeting another.
	Platform Platform
}

// New builds a resolver from a resolved specification and the data embedded
// alongside it.
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
		spec:       spec,
		cache:      cache,
		platform:   platform,
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

// Resolve materializes a named resource.
func (r *Resolver) Resolve(ctx context.Context, resourceID string) (string, error) {
	def, ok := r.spec[resourceID]
	if !ok {
		return "", fmt.Errorf("%w: unknown runtime artifact %q", ErrUnknownMember, resourceID)
	}

	return r.resolveDefinition(ctx, def, resourceID)
}

// ResolveDescriptor materializes an already selected artifact.
func (r *Resolver) ResolveDescriptor(ctx context.Context, descriptor Descriptor) (string, error) {
	return r.resolveDefinition(ctx, ResourceDefinition{
		Extract: descriptor.Extract,
		Artifact: map[string]ArtifactSpec{"any": {
			URL: descriptor.Locator.URL, Ref: descriptor.Locator.Ref,
			Sha256: descriptor.Sha256, Subpath: descriptor.Subpath,
			DownloadPath: descriptor.DownloadPath,
		}},
	}, descriptor.Locator.String())
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

// List returns declared resource IDs beginning with prefix.
func (r *Resolver) List(prefix string) []string {
	ids := make([]string, 0, len(r.spec))
	for resourceID := range r.spec {
		if strings.HasPrefix(resourceID, prefix) {
			ids = append(ids, resourceID)
		}
	}
	slices.Sort(ids)

	return ids
}

// Cache returns the cache this resolver stores resources in.
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

// contentIdentity is what the cache keys on. A declared checksum is a content
// hash, so it doubles as an identity; anything else comes from the source, and
// an empty result means nothing can identify this content and it is refreshed
// on every request.
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
