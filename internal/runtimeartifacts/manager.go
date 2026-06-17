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
	"strings"
)

const (
	dirPerm        = 0o700
	filePerm       = 0o600
	extractRelPath = "unpack"
)

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
	&FileSource{},
	&HttpSource{},
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
	if strings.TrimSpace(artifact.Sha256) == "" {
		artifact.Sha256 = m.identify(ctx, artifact)
	}

	entry, err := m.resolveEntry(resourceID, def, artifact)
	if err != nil {
		return "", err
	}

	noChecksum := strings.TrimSpace(artifact.Sha256) == ""

	var resolvedPath string
	err = m.cache.withExclusiveLock(ctx, func() error {
		index, _, err := m.cache.readIndex()
		if err != nil {
			return err
		}

		if noChecksum {
			slog.Info(
				"re-fetching resource without checksum, result may not be stable",
				"id",
				resourceID,
				"url",
				artifact.URL,
			)
		} else {
			cachedPath, err := m.getCacheEntry(&index, artifact, entry)
			if err != nil {
				return err
			}
			if cachedPath != "" {
				resolvedPath = cachedPath
				slog.Info("found resource in cache", "id", resourceID, "path", resolvedPath)

				return nil
			}
		}

		resolvedPath, err = m.refresh(ctx, resourceID, artifact, &entry, &index)
		if err != nil {
			return err
		}

		slog.Info("fetched resource", "id", resourceID, "path", resolvedPath)

		return nil
	})
	if err != nil {
		return "", err
	}

	return resolvedPath, nil
}

// Request looks up a definition from the static spec by ID and resolves it.
func (m *Manager) Request(ctx context.Context, resourceID string) (string, error) {
	def, ok := m.spec[resourceID]
	if !ok {
		return "", fmt.Errorf("unknown runtime artifact %q", resourceID)
	}

	return m.Get(ctx, def, resourceID)
}

func (*Manager) identify(ctx context.Context, artifact ArtifactSpec) string {
	for _, src := range sources {
		if !src.CanFetch(artifact.URL) {
			continue
		}
		if id, ok := src.(Identifier); ok {
			hash, err := id.Identify(ctx, artifact.URL)
			if err != nil {
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

		info, err := os.Stat(fetchPath)
		if err != nil {
			return err
		}

		// Only check the checksum for files with a specified sha256.
		if !info.IsDir() && strings.TrimSpace(artifact.Sha256) != "" {
			actual, err := sha256OfFile(fetchPath)
			if err != nil {
				return err
			}
			if actual != artifact.Sha256 {
				return checksumMismatchError(artifact.Sha256, actual)
			}
		}

		return nil
	}

	return fmt.Errorf("unsupported resource scheme in %q", artifact.URL)
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

	if entry.RedirectPath != "" && !entry.Extract {
		return entry.RedirectPath, nil
	}

	return m.cache.absolutePath(entry.ResolvedPath), nil
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
	if err := m.fetch(ctx, artifact, entry); err != nil {
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

	if entry.RedirectPath != "" && !entry.Extract {
		return entry.RedirectPath, nil
	}

	return m.cache.absolutePath(entry.ResolvedPath), nil
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

	platform := platformKey(m.platform.GOOS, m.platform.GOARCH)
	cacheKey, err := artifactIdentity(
		resourceID,
		platform,
		def.Extract,
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

	resolvedRelPath := artifactRelPath
	if def.Extract {
		resolvedAbsPath, err := extractedResourcePath(
			filepath.Join(entryPath, extractRelPath),
			resourcePath,
		)
		if err != nil {
			return cacheIndexEntry{}, err
		}
		resolvedRelPath, err = m.cache.relativePath(resolvedAbsPath)
		if err != nil {
			return cacheIndexEntry{}, err
		}
	}

	return cacheIndexEntry{
		Key:          cacheKey,
		Platform:     platform,
		EntryPath:    entryRelPath,
		ArtifactPath: artifactRelPath,
		ResolvedPath: resolvedRelPath,
		DownloadPath: downloadPath,
		ResourcePath: resourcePath,
		Extract:      def.Extract,
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

func extractedResourcePath(extractedRoot, resourcePath string) (string, error) {
	if strings.TrimSpace(resourcePath) == "" {
		return extractedRoot, nil
	}

	return pathWithinRoot(extractedRoot, resourcePath, "resource_path")
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
	if strings.HasPrefix(rawPath, "file://") {
		rawPath = strings.TrimPrefix(rawPath, "file://")
	} else if idx := strings.Index(rawPath, "://"); idx >= 0 {
		rawPath = rawPath[idx+3:]
	}
	base := filepath.Base(rawPath)
	if base == "." || base == "/" {
		return ""
	}

	return base
}
