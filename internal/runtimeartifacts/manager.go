// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package runtimeartifacts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
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

type Source interface {
	CanFetch(url string) bool
	// Fetch downloads or copies the resource at url to dstPath. It returns a
	// non-empty redirectPath when the resource already resides locally and
	// dstPath should be ignored; the caller uses redirectPath directly. For all
	// other sources redirectPath is empty.
	Fetch(ctx context.Context, url, dstPath string) (redirectPath string, err error)
}

// Identifier is an optional interface for Source types that can resolve their
// content identity before fetching. The returned string is used as a synthetic
// Sha256 so the standard cache machinery handles deduplication.
type Identifier interface {
	Identify(ctx context.Context, url string) (string, error)
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

type artifactIdentityPayload struct {
	ResourceID   string `json:"resourceId"`
	Platform     string `json:"platform"`
	URL          string `json:"url"`
	Sha256       string `json:"sha256"`
	CommitHash   string `json:"commitHash,omitempty"`
	Extract      bool   `json:"extract"`
	DownloadPath string `json:"downloadPath"`
	ResourcePath string `json:"resourcePath"`
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

// Get resolves an artifact from a runtime-constructed definition.
func (m *Manager) Get(
	ctx context.Context,
	def ResourceDefinition,
	resourceID string,
) (string, error) {
	artifact, err := def.Resolve(m.platform.GOOS, m.platform.GOARCH)
	if err != nil {
		return "", err
	}

	// If the source can identify its content before fetching, use that identity
	// as a synthetic Sha256 so the standard cache machinery handles the rest.
	// An embed: true resource uses its own build-time content hash instead,
	// even when the source artifact also declares a checksum: the checksum
	// describes what was fetched from the source, not necessarily what a
	// particular build ends up embedding, so only the build's own hash can
	// tell two builds' embedded content apart. A source's own Identify exists
	// to identify network content before fetching it, not to stand in for the
	// identity of whatever was actually embedded (e.g. FileSource.Identify
	// hashes a path, not file content).
	switch {
	case def.Embed:
		if hash, ok := lookupEmbeddedHash(resourceID); ok {
			artifact.Sha256 = hash
		}
	case strings.TrimSpace(artifact.Sha256) == "":
		artifact.Sha256 = m.identify(ctx, artifact)
	default:
	}

	entry, err := m.resolveEntry(resourceID, def, artifact)
	if err != nil {
		return "", err
	}

	var resolvedPath string
	err = m.cache.withExclusiveLock(ctx, func() error {
		var lockErr error
		resolvedPath, lockErr = m.resolveUnderLock(ctx, resourceID, artifact, &entry)

		return lockErr
	})
	if err != nil {
		return "", err
	}

	return resolvedPath, nil
}

// Request looks up a definition from the static spec by ID and resolves it.
func (m *Manager) Request(ctx context.Context, resourceID string) (string, error) {
	return m.RequestMember(ctx, resourceID, "")
}

// RequestMember treats an empty member as plain Request. Otherwise it
// matches member against the resolved group's own resource_path pattern:
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
	matches, err := GlobMatches(root, artifact.ResourcePath)
	if err != nil {
		return "", err
	}
	path, ok := matches[member]
	if !ok {
		return "", fmt.Errorf("%w %q of group %q", ErrUnknownMember, member, resourceID)
	}

	return path, nil
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
		return nil, errors.New("must define resource_path with a glob pattern")
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

// resolveUnderLock resolves the cached path for an artifact, refreshing it if
// necessary. It must run under the cache's exclusive lock since it reads and
// mutates the shared index.
func (m *Manager) resolveUnderLock(
	ctx context.Context,
	resourceID string,
	artifact ArtifactSpec,
	entry *cacheIndexEntry,
) (string, error) {
	index, _, err := m.cache.readIndex()
	if err != nil {
		return "", err
	}

	if strings.TrimSpace(artifact.Sha256) == "" {
		slog.Info(
			"re-fetching resource without checksum, result may not be stable",
			"id",
			resourceID,
			"url",
			artifact.URL,
		)
	} else {
		cachedPath, err := m.getCacheEntry(&index, artifact, *entry)
		if err != nil {
			return "", err
		}
		if cachedPath != "" {
			slog.Info("found resource in cache", "id", resourceID, "path", cachedPath)

			return cachedPath, nil
		}
	}

	resolvedPath, err := m.refresh(ctx, resourceID, artifact, entry, &index)
	if err != nil {
		return "", err
	}

	slog.Info("fetched resource", "id", resourceID, "path", resolvedPath)

	return resolvedPath, nil
}

func (*Manager) identify(ctx context.Context, artifact ArtifactSpec) string {
	for _, src := range sources {
		if !src.CanFetch(artifact.URL) {
			continue
		}
		if id, ok := src.(Identifier); ok {
			if hash, err := id.Identify(ctx, artifact.URL); err == nil {
				return hash
			}
		}

		break
	}

	return ""
}

func (m *Manager) fetch(ctx context.Context, artifact ArtifactSpec, entry *cacheIndexEntry) error {
	for _, source := range sources {
		if !source.CanFetch(artifact.URL) {
			continue
		}

		return m.fetchFromSource(ctx, source, artifact, entry)
	}

	return fmt.Errorf("unsupported resource scheme in %q", artifact.URL)
}

func (m *Manager) fetchFromSource(
	ctx context.Context,
	source Source,
	artifact ArtifactSpec,
	entry *cacheIndexEntry,
) error {
	fetchPath := m.cache.absolutePath(entry.ArtifactPath)

	_ = os.MkdirAll(filepath.Dir(fetchPath), dirPerm)
	redirectPath, err := source.Fetch(ctx, artifact.URL, fetchPath)
	if err != nil {
		return err
	}
	if redirectPath != "" {
		entry.RedirectPath = redirectPath

		return nil
	}

	return verifyFetchedChecksum(fetchPath, artifact.Sha256)
}

func verifyFetchedChecksum(fetchPath, expectedSha256 string) error {
	info, err := os.Stat(fetchPath)
	if err != nil {
		return err
	}

	// Only check the checksum for files with a specified sha256.
	if info.IsDir() || strings.TrimSpace(expectedSha256) == "" {
		return nil
	}

	actual, err := sha256OfFile(fetchPath)
	if err != nil {
		return err
	}
	if actual != expectedSha256 {
		return checksumMismatchError(expectedSha256, actual)
	}

	return nil
}

// resolveEmbedded materializes an embed:true resource from data compiled into
// the binary. A registry miss is a hard error, never a fallback to the
// network sources list — see design.md for why.
func (m *Manager) resolveEmbedded(resourceID string, entry *cacheIndexEntry) error {
	data, ok := lookupEmbedded(resourceID)
	if !ok {
		return fmt.Errorf("no embedded data registered for resource %q", resourceID)
	}

	fetchPath := m.cache.absolutePath(entry.ArtifactPath)
	if err := os.MkdirAll(filepath.Dir(fetchPath), dirPerm); err != nil {
		return err
	}
	if err := os.WriteFile(fetchPath, data, filePerm); err != nil {
		return err
	}

	slog.Info("resolved resource from embedded data", "id", resourceID)

	return nil
}

func (m *Manager) extract(entry cacheIndexEntry) error {
	filename := m.cache.absolutePath(entry.ArtifactPath)
	if entry.RedirectPath != "" {
		filename = entry.RedirectPath
	}

	for _, extractor := range extractors {
		if extractor.CanExtract(filename) {
			extractPath := filepath.Join(m.cache.absolutePath(entry.EntryPath), extractRelPath)

			_ = os.MkdirAll(extractPath, dirPerm)

			return extractor.Extract(filename, extractPath)
		}
	}

	return fmt.Errorf("unsupported resource archive format in %q", filename)
}

func (m *Manager) getCacheEntry(
	index *cacheIndex,
	artifact ArtifactSpec,
	target cacheIndexEntry,
) (string, error) {
	entry, ok := index.Entries[target.Key]
	if !ok {
		return "", nil
	}

	if entry.URL != artifact.URL {
		return "", nil
	}
	if entry.Sha256 != normalizeSha256(artifact.Sha256) {
		return "", nil
	}
	if entry.EntryPath != target.EntryPath ||
		entry.ArtifactPath != target.ArtifactPath ||
		entry.ResolvedPath != target.ResolvedPath {
		return "", nil
	}

	pathToStat := m.cache.absolutePath(entry.ResolvedPath)
	if entry.RedirectPath != "" && !entry.Extract {
		pathToStat = entry.RedirectPath
	}
	if _, err := os.Stat(pathToStat); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}

		return "", err
	}

	entry.LastUsedAt = m.cache.clock.Now().UTC()
	if size, statErr := directorySize(m.cache.absolutePath(target.EntryPath)); statErr == nil {
		entry.SizeBytes = size
	}
	index.Entries[target.Key] = entry
	if err := m.writeIndexAfterCleanup(index); err != nil {
		return "", err
	}

	return m.returnPath(entry)
}

func (m *Manager) writeIndexAfterCleanup(index *cacheIndex) error {
	if err := m.cache.cleanupStaleIfDue(index); err != nil {
		slog.Warn("failed to clean runtime artifact cache; continuing", "error", err)
	}

	return m.cache.writeIndex(*index)
}

func (m *Manager) refresh(
	ctx context.Context,
	resourceID string,
	artifact ArtifactSpec,
	entry *cacheIndexEntry,
	index *cacheIndex,
) (string, error) {
	if entry.Embed {
		if err := m.resolveEmbedded(resourceID, entry); err != nil {
			return "", err
		}
	} else if err := m.fetch(ctx, artifact, entry); err != nil {
		return "", errors.Join(fmt.Errorf("failed to fetch resource %q", resourceID), err)
	}

	if entry.Extract {
		if err := m.extract(*entry); err != nil {
			return "", errors.Join(fmt.Errorf("failed to extract resource %q", resourceID), err)
		}
	}

	size, err := directorySize(m.cache.absolutePath(entry.EntryPath))
	if err != nil {
		size = 0
	}

	now := m.cache.clock.Now().UTC()
	entry.ResourceID = resourceID
	entry.URL = artifact.URL
	entry.Sha256 = normalizeSha256(artifact.Sha256)
	entry.CreatedAt = now
	entry.LastUsedAt = now
	entry.SizeBytes = size
	index.Entries[entry.Key] = *entry

	if err := m.writeIndexAfterCleanup(index); err != nil {
		return "", err
	}

	return m.returnPath(*entry)
}

// resolveEntry computes the cache slot for a resource: the cache key, relative
// paths for the entry directory, downloaded artifact, and resolved artifact.
// Fields populated at later pipeline stages (ResourceID, URL, Sha256,
// RedirectPath, timestamps) are left at their zero values.
func (m *Manager) resolveEntry(
	resourceID string,
	def ResourceDefinition,
	artifact ArtifactSpec,
) (cacheIndexEntry, error) {
	downloadPath, err := cleanRelativePath(artifact.DownloadPath, "download_path")
	if err != nil {
		return cacheIndexEntry{}, err
	}
	if downloadPath == "" {
		downloadPath = urlBasename(artifact.URL)
	}

	resourcePath, err := cleanRelativePath(artifact.ResourcePath, "resource_path")
	if err != nil {
		return cacheIndexEntry{}, err
	}
	if def.Glob {
		// resource_path is a match pattern applied after Get returns here,
		// not a literal subdirectory to select.
		resourcePath = ""
	}

	// A Glob:true resource's embedded bytes are always an archive of its
	// matched entries, even when the group's own declared extract is false
	// because its live source is already a bare directory needing no
	// extraction to read.
	extract := def.Extract
	if def.Glob && def.Embed {
		extract = true
	}

	platform := platformKey(m.platform.GOOS, m.platform.GOARCH)
	cacheKey, err := artifactIdentity(
		resourceID,
		platform,
		extract,
		artifact,
		downloadPath,
		resourcePath,
	)
	if err != nil {
		return cacheIndexEntry{}, err
	}

	entryPath := filepath.Join(m.cache.artifactsRoot(), resourceID, platform, cacheKey)

	entryRelPath, err := m.cache.relativePath(entryPath)
	if err != nil {
		return cacheIndexEntry{}, err
	}

	artifactAbsPath, err := pathWithinRoot(entryPath, downloadPath, "download_path")
	if err != nil {
		return cacheIndexEntry{}, err
	}

	artifactRelPath, err := m.cache.relativePath(artifactAbsPath)
	if err != nil {
		return cacheIndexEntry{}, err
	}

	resolvedRelPath, err := m.resolveResolvedPath(
		extract, entryPath, artifactAbsPath, artifactRelPath, resourcePath,
	)
	if err != nil {
		return cacheIndexEntry{}, err
	}

	return cacheIndexEntry{
		Key:          cacheKey,
		Platform:     platform,
		EntryPath:    entryRelPath,
		ArtifactPath: artifactRelPath,
		ResolvedPath: resolvedRelPath,
		DownloadPath: downloadPath,
		ResourcePath: resourcePath,
		Extract:      extract,
		Embed:        def.Embed,
	}, nil
}

func artifactIdentity(
	resourceID, platform string,
	extract bool,
	artifact ArtifactSpec,
	downloadPath, resourcePath string,
) (string, error) {
	payload := artifactIdentityPayload{
		ResourceID:   resourceID,
		Platform:     platform,
		URL:          artifact.URL,
		Sha256:       normalizeSha256(artifact.Sha256),
		Extract:      extract,
		DownloadPath: downloadPath,
		ResourcePath: resourcePath,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)

	return hex.EncodeToString(sum[:]), nil
}

// returnPath computes the effective path returned to callers of Get. For
// redirect sources (file:// directories or bare files) the returned path is
// the redirect itself; when resource_path is set, it selects a subdirectory of
// the redirect target. For cached artifacts, ResolvedPath already encodes the
// resource_path suffix (see resolveResolvedPath).
func (m *Manager) returnPath(entry cacheIndexEntry) (string, error) {
	if entry.RedirectPath != "" && !entry.Extract {
		if strings.TrimSpace(entry.ResourcePath) == "" {
			return entry.RedirectPath, nil
		}

		return pathWithinRoot(entry.RedirectPath, entry.ResourcePath, "resource_path")
	}

	return m.cache.absolutePath(entry.ResolvedPath), nil
}

// resolveResolvedPath computes the resolved path returned to callers of Get.
// The root is the extracted directory for archives that are extracted, and the
// artifact path itself otherwise. A non-empty resource_path selects a
// subdirectory within that root, applying uniformly regardless of source kind.
func (m *Manager) resolveResolvedPath(
	extract bool,
	entryPath, artifactAbsPath, artifactRelPath, resourcePath string,
) (string, error) {
	root := artifactAbsPath
	if extract {
		root = filepath.Join(entryPath, extractRelPath)
	}
	if strings.TrimSpace(resourcePath) == "" {
		if extract {
			return m.cache.relativePath(root)
		}

		return artifactRelPath, nil
	}
	resolvedAbsPath, err := pathWithinRoot(root, resourcePath, "resource_path")
	if err != nil {
		return "", err
	}

	return m.cache.relativePath(resolvedAbsPath)
}

func normalizeSha256(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func checksumMismatchError(expected, actual string) error {
	return fmt.Errorf(
		"checksum mismatch: expected %s, got %s",
		expected,
		actual,
	)
}

func urlBasename(rawURL string) string {
	rawPath := rawURL
	if after, ok := strings.CutPrefix(rawPath, "file://"); ok {
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
