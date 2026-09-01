// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package resource

import (
	"context"
	"errors"
	"fmt"
	"io"
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

var sources = []Source{
	&GitSource{},
	&FileSource{},
	&HttpSource{},
	&DockerSource{},
}

var extractors = []Extractor{
	&TarGzExtractor{},
	&ZipExtractor{},
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

// RequestMember treats an empty member as plain Request. Otherwise it
// matches member against the resolved group's own subpath pattern:
// never an independent fetch, cache entry, or embed, only a subpath of the
// already-resolved group.
func (m *Manager) RequestMember(ctx context.Context, resourceID, member string) (string, error) {
	def, ok := m.spec[resourceID]
	if !ok {
		return "", fmt.Errorf("%w: unknown runtime artifact %q", ErrUnknownMember, resourceID)
	}
	if member == "" {
		return m.Get(ctx, def, resourceID)
	}

	artifact, err := def.Resolve(m.platform.GOOS, m.platform.GOARCH)
	if err != nil {
		return "", err
	}
	root, err := m.Get(ctx, def, resourceID)
	if err != nil {
		return "", err
	}
	matches, err := GlobMatches(root, artifact.Subpath)
	if err != nil {
		return "", err
	}
	path, ok := matches[member]
	if !ok {
		return "", fmt.Errorf("%w %q of group %q", ErrUnknownMember, member, resourceID)
	}

	return path, nil
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

// GroupMembers reports the names matched at build time, without
// re-globbing or extracting the embedded archive live.
func (*Manager) GroupMembers(group string) []string {
	members, _ := lookupEmbeddedGroupMembers(group)

	return slices.Clone(members)
}

// GlobMatches always excludes a matched ".git" directory, since a pattern
// of "*" at a cloned repository's own root would otherwise match its
// metadata directory like any other top-level entry.
func GlobMatches(root, pattern string) (map[string]string, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return nil, errors.New("must define subpath with a glob pattern")
	}

	globMatches, err := filepath.Glob(filepath.Join(root, pattern))
	if err != nil {
		return nil, err
	}

	matches := make(map[string]string, len(globMatches))
	for _, match := range globMatches {
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

func sourceFor(loc Locator) (Source, error) {
	for _, source := range sources {
		if source.Handles(loc) {
			return source, nil
		}
	}

	return nil, fmt.Errorf("unsupported resource scheme in %q", loc.URL)
}

func probeSource(ctx context.Context, loc Locator) (Probe, error) {
	source, err := sourceFor(loc)
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
