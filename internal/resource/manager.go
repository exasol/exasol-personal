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

// ErrUnknownMember marks a Request/RequestMember/RequestMemberCopy failure
// caused by nothing matching what was asked for: an undeclared resourceID,
// or a member not matching its group's pattern. It is deliberately distinct
// from a resolution failure (fetch, cache I/O, extraction) for a resourceID
// or member that does exist. Callers that translate failures into a
// caller-facing "does not exist" error must check for this specifically,
// not any error.
var ErrUnknownMember = errors.New("unknown member")

type Platform struct {
	GOOS   string
	GOARCH string
}

type Manager struct {
	spec     ResourceSpec
	cache    *Cache
	platform Platform
	sources  []Source
}

// Probe is what a source can determine about a locator without transferring
// its content.
type Probe struct {
	// Identity changes exactly when the content changes. It is opaque to the
	// manager, which only keys the cache on it; each source formats its own.
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

var extractors = []Extractor{
	&TarGzExtractor{},
	&ZipExtractor{},
}

// Options describes the resolver a caller wants.
type Options struct {
	// Spec is the resolved resource specification, as YAML.
	Spec []byte
	// Blobs holds data compiled into the binary, addressed by the paths Spec
	// names. Nil means nothing is embedded and every resource resolves from
	// its upstream source.
	Blobs fs.FS
	// CacheRoot overrides where resources are stored. Empty uses the per-user
	// default.
	CacheRoot string
	// Platform overrides the host platform, for a build targeting another.
	Platform Platform
}

// New builds a resolver from a resolved specification and the data embedded
// alongside it.
func New(opts Options) (*Manager, error) {
	spec, err := ParseSpec(opts.Spec)
	if err != nil {
		return nil, err
	}

	cache := opts.cache()
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

	return &Manager{
		spec:     spec,
		cache:    cache,
		platform: platform,
		sources:  defaultSources(opts.Blobs),
	}, nil
}

func (o Options) cache() *Cache {
	if strings.TrimSpace(o.CacheRoot) == "" {
		return nil
	}

	return NewCache(o.CacheRoot, filepath.Join(o.CacheRoot, cacheConfigFileName))
}

// NewManager creates a Manager backed by the default cache.
// It has no resource spec, so only Get (ad-hoc definitions) is available.
func NewManager() (*Manager, error) {
	cache, err := NewDefaultCache()
	if err != nil {
		return nil, err
	}

	return NewResourceManagerWithCache(ResourceSpec{}, cache), nil
}

func NewResourceManager(spec ResourceSpec) (*Manager, error) {
	cache, err := NewDefaultCache()
	if err != nil {
		return nil, err
	}

	return NewResourceManagerWithCache(spec, cache), nil
}

// NewResourceManagerWithSpec parses rawSpec as a resource specification and
// returns a Manager backed by the default cache.
func NewResourceManagerWithSpec(rawSpec []byte) (*Manager, error) {
	spec, err := ParseSpec(rawSpec)
	if err != nil {
		return nil, err
	}

	return NewResourceManager(spec)
}

// NewResourceManagerWithSpecForPlatform mirrors NewResourceManagerWithSpec
// for an explicit target platform rather than the host's own.
func NewResourceManagerWithSpecForPlatform(
	rawSpec []byte, cacheRoot, goos, goarch string,
) (*Manager, error) {
	spec, err := ParseSpec(rawSpec)
	if err != nil {
		return nil, err
	}

	return NewResourceManagerForPlatform(spec, cacheRoot, goos, goarch), nil
}

func NewResourceManagerWithCache(spec ResourceSpec, cache *Cache) *Manager {
	return NewResourceManagerWithCacheForPlatform(spec, cache, runtime.GOOS, runtime.GOARCH)
}

func NewResourceManagerWithCacheForPlatform(
	spec ResourceSpec,
	cache *Cache,
	goos, goarch string,
) *Manager {
	return &Manager{
		spec:  spec,
		cache: cache,
		platform: Platform{
			GOOS:   goos,
			GOARCH: goarch,
		},
		sources: defaultSources(nil),
	}
}

func NewResourceManagerForPlatform(spec ResourceSpec, cacheRoot, goos, goarch string) *Manager {
	return NewResourceManagerWithCacheForPlatform(
		spec,
		NewCache(cacheRoot, filepath.Join(cacheRoot, cacheConfigFileName)),
		goos,
		goarch,
	)
}

// Cache returns the cache this manager stores resources in.
func (m *Manager) Cache() *Cache {
	return m.cache
}

// Request looks up a definition from the static spec by ID and resolves it.
func (m *Manager) Request(ctx context.Context, resourceID string) (string, error) {
	return m.RequestMember(ctx, resourceID, "")
}

// RequestMember treats an empty member as plain Request. Otherwise it resolves
// the member as the resource it is, named "<group>/<member>".
func (m *Manager) RequestMember(ctx context.Context, resourceID, member string) (string, error) {
	if member != "" {
		if _, ok := m.spec[resourceID+"/"+member]; !ok {
			return "", fmt.Errorf(
				"%w %q of group %q", ErrUnknownMember, member, resourceID,
			)
		}

		resourceID += "/" + member
	}

	def, ok := m.spec[resourceID]
	if !ok {
		return "", fmt.Errorf("%w: unknown runtime artifact %q", ErrUnknownMember, resourceID)
	}

	return m.Get(ctx, def, resourceID)
}

// RequestCopy is RequestMemberCopy with an empty member.
func (m *Manager) RequestCopy(ctx context.Context, resourceID, destDir string) error {
	return m.RequestMemberCopy(ctx, resourceID, "", destDir)
}

// RequestMemberCopy mirrors RequestCopy, resolving via RequestMember.
func (m *Manager) RequestMemberCopy(ctx context.Context, resourceID, member, destDir string) error {
	path, err := m.RequestMember(ctx, resourceID, member)
	if err != nil {
		return err
	}

	return copyResolvedPath(path, destDir)
}

// GetCopy mirrors RequestCopy for a runtime-constructed definition, the way
// Get mirrors Request.
func (m *Manager) GetCopy(
	ctx context.Context, def ResourceDefinition, resourceID, destDir string,
) error {
	path, err := m.Get(ctx, def, resourceID)
	if err != nil {
		return err
	}

	return copyResolvedPath(path, destDir)
}

func copyResolvedPath(path, destDir string) error {
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

// GroupMembers reports the members a group declares. A glob is expanded at
// build time into resources named "<group>/<member>", so membership is read
// from the specification rather than matched, extracted, or registered.
func (m *Manager) GroupMembers(group string) []string {
	prefix := group + "/"
	members := make([]string, 0, len(m.spec))
	for resourceID := range m.spec {
		if name, ok := strings.CutPrefix(resourceID, prefix); ok && name != "" {
			members = append(members, name)
		}
	}
	slices.Sort(members)

	return members
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

func (m *Manager) sourceFor(loc Locator) (Source, error) {
	for _, source := range m.sources {
		if source.Handles(loc) {
			return source, nil
		}
	}

	return nil, fmt.Errorf("unsupported resource scheme in %q", loc.URL)
}

func (m *Manager) probeSource(ctx context.Context, loc Locator) (Probe, error) {
	source, err := m.sourceFor(loc)
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
