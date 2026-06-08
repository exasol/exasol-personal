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
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	dirPerm  = 0o700
	filePerm = 0o600
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

type artifactRequest struct {
	cacheKey        string
	platform        string
	entryRelPath    string
	artifactRelPath string
	resolvedRelPath string
	downloadPath    string
	resourcePath    string
	extract         bool
}

func (r artifactRequest) entryPath(cache *Cache) string {
	return cache.absolutePath(r.entryRelPath)
}

func (r artifactRequest) artifactPath(cache *Cache) string {
	return cache.absolutePath(r.artifactRelPath)
}

func (r artifactRequest) resolvedPath(cache *Cache) string {
	return cache.absolutePath(r.resolvedRelPath)
}

func (r artifactRequest) withEntryPath(cache *Cache, entryPath string) (artifactRequest, error) {
	var err error
	r.entryRelPath, err = cache.relativePath(entryPath)
	if err != nil {
		return artifactRequest{}, err
	}
	artifactPath, err := pathWithinRoot(entryPath, r.downloadPath, "download_path")
	if err != nil {
		return artifactRequest{}, err
	}
	r.artifactRelPath, err = cache.relativePath(artifactPath)
	if err != nil {
		return artifactRequest{}, err
	}
	r.resolvedRelPath = r.artifactRelPath
	if r.extract {
		extractedRoot, err := archiveExtractionRoot(r.artifactPath(cache))
		if err != nil {
			return artifactRequest{}, err
		}
		resolvedPath, err := extractedResourcePath(extractedRoot, r.resourcePath)
		if err != nil {
			return artifactRequest{}, err
		}
		r.resolvedRelPath, err = cache.relativePath(resolvedPath)
		if err != nil {
			return artifactRequest{}, err
		}
	}

	return r, nil
}

type artifactIdentityPayload struct {
	ResourceID   string `json:"resourceId"`
	Platform     string `json:"platform"`
	URL          string `json:"url"`
	Sha256       string `json:"sha256"`
	Extract      bool   `json:"extract"`
	DownloadPath string `json:"downloadPath"`
	ResourcePath string `json:"resourcePath"`
}

func NewResourceManager(spec ResourceSpec) (*Manager, error) {
	cache, err := NewDefaultCache()
	if err != nil {
		return nil, err
	}

	return NewResourceManagerWithCache(spec, cache), nil
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

func (m *Manager) Request(ctx context.Context, resourceID string) (string, error) {
	def, ok := m.spec[resourceID]
	if !ok {
		return "", fmt.Errorf("unknown runtime artifact %q", resourceID)
	}

	artifact, err := def.Resolve(m.platform.GOOS, m.platform.GOARCH)
	if err != nil {
		return "", err
	}
	target, err := m.resolveArtifactRequest(resourceID, def, artifact)
	if err != nil {
		return "", err
	}

	var resolvedPath string
	err = m.cache.withExclusiveLock(ctx, func() error {
		index, _, err := m.cache.readIndex()
		if err != nil {
			return err
		}

		path, reused, err := m.reuseCacheEntry(&index, artifact, target)
		if err != nil {
			return err
		}
		if reused {
			resolvedPath = path
			slog.Debug("found resource in cache", "id", resourceID, "path", resolvedPath)

			return nil
		}

		path, err = m.refresh(ctx, resourceID, artifact, target, &index)
		if err != nil {
			return err
		}
		if err := m.writeIndexAfterCleanup(&index); err != nil {
			return err
		}
		resolvedPath = path
		slog.Debug("downloaded resource", "id", resourceID, "path", resolvedPath)

		return nil
	})
	if err != nil {
		return "", err
	}

	return resolvedPath, nil
}

func (m *Manager) reuseCacheEntry(
	index *cacheIndex,
	artifact ArtifactSpec,
	target artifactRequest,
) (string, bool, error) {
	entry, ok := index.Entries[target.cacheKey]
	if !ok {
		return "", false, nil
	}
	resolvedPath, reusable, err := cacheEntryPathIfReusable(entry, artifact, target, m.cache)
	if err != nil {
		return "", false, err
	}
	if !reusable {
		return "", false, nil
	}

	entry.LastUsedAt = m.cache.clock.Now().UTC()
	if size, statErr := directorySize(target.entryPath(m.cache)); statErr == nil {
		entry.SizeBytes = size
	}
	index.Entries[target.cacheKey] = entry
	if err := m.writeIndexAfterCleanup(index); err != nil {
		return "", false, err
	}

	return resolvedPath, true, nil
}

func (m *Manager) cleanupStaleBestEffort(index *cacheIndex) {
	if err := m.cache.cleanupStaleIfDue(index); err != nil {
		slog.Warn("failed to clean runtime artifact cache; continuing", "error", err)
	}
}

func (m *Manager) writeIndexAfterCleanup(index *cacheIndex) error {
	m.cleanupStaleBestEffort(index)

	return m.cache.writeIndex(*index)
}

func cacheEntryPathIfReusable(
	entry cacheIndexEntry,
	artifact ArtifactSpec,
	target artifactRequest,
	cache *Cache,
) (string, bool, error) {
	expected := normalizeSha256(artifact.Sha256)
	if entry.URL != artifact.URL {
		return "", false, nil
	}
	if entry.Sha256 != expected {
		return "", false, nil
	}
	if entry.EntryPath != target.entryRelPath ||
		entry.ArtifactPath != target.artifactRelPath ||
		entry.ResolvedPath != target.resolvedRelPath {
		return "", false, nil
	}

	artifactPath := target.artifactPath(cache)
	if _, err := os.Stat(artifactPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}

		return "", false, err
	}

	if !target.extract {
		return artifactPath, true, nil
	}

	resolvedPath := target.resolvedPath(cache)
	if _, err := os.Stat(resolvedPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}

		return "", false, err
	}

	return resolvedPath, true, nil
}

func (m *Manager) refresh(
	ctx context.Context,
	resourceID string,
	artifact ArtifactSpec,
	target artifactRequest,
	index *cacheIndex,
) (string, error) {
	stage, cleanup, err := m.stageArtifactRequest(target)
	if err != nil {
		return "", err
	}
	committed := false
	defer func() {
		if !committed {
			cleanup()
		}
	}()

	if err := downloadArtifact(
		ctx,
		resourceID,
		artifact.URL,
		stage.artifactPath(m.cache),
		artifact.Sha256,
	); err != nil {
		slog.Error("failed to download resource", "id", resourceID, "error", err)
		return "", err
	}

	if stage.extract {
		_, err := extractArtifact(stage.artifactPath(m.cache), artifact.ResourcePath)
		if err != nil {
			return "", err
		}
	}

	size, err := directorySize(stage.entryPath(m.cache))
	if err != nil {
		return "", err
	}
	if err := os.RemoveAll(target.entryPath(m.cache)); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(target.entryPath(m.cache)), dirPerm); err != nil {
		return "", err
	}
	if err := os.Rename(stage.entryPath(m.cache), target.entryPath(m.cache)); err != nil {
		return "", err
	}
	committed = true

	now := m.cache.clock.Now().UTC()
	index.Entries[target.cacheKey] = cacheIndexEntry{
		ResourceID:   resourceID,
		Platform:     target.platform,
		URL:          artifact.URL,
		Sha256:       normalizeSha256(artifact.Sha256),
		Extract:      target.extract,
		DownloadPath: target.downloadPath,
		ResourcePath: target.resourcePath,
		EntryPath:    target.entryRelPath,
		ArtifactPath: target.artifactRelPath,
		ResolvedPath: target.resolvedRelPath,
		CreatedAt:    now,
		LastUsedAt:   now,
		SizeBytes:    size,
	}

	if target.extract {
		return target.resolvedPath(m.cache), nil
	}

	return target.artifactPath(m.cache), nil
}

func (m *Manager) stageArtifactRequest(target artifactRequest) (artifactRequest, func(), error) {
	parent := m.cache.downloadsRoot()
	if err := os.MkdirAll(parent, dirPerm); err != nil {
		return artifactRequest{}, nil, err
	}
	stageEntryPath, err := os.MkdirTemp(parent, ".tmp-"+target.cacheKey+"-")
	if err != nil {
		return artifactRequest{}, nil, err
	}
	staged := false
	defer func() {
		if !staged {
			_ = os.RemoveAll(stageEntryPath)
		}
	}()

	stage, err := target.withEntryPath(m.cache, stageEntryPath)
	if err != nil {
		return artifactRequest{}, nil, err
	}

	staged = true

	return stage, func() { _ = os.RemoveAll(stageEntryPath) }, nil
}

func (a ArtifactSpec) downloadPath() (string, error) {
	if name := strings.TrimSpace(a.DownloadPath); name != "" {
		return name, nil
	}

	parsed, err := url.Parse(a.URL)
	if err != nil {
		return "", err
	}

	name := path.Base(parsed.Path)
	if name == "." || name == "/" || name == "" {
		return "", fmt.Errorf(
			"resource URL %q must end with a filename or provide download_path",
			a.URL,
		)
	}

	return name, nil
}

func (m *Manager) resolveArtifactRequest(
	resourceID string,
	def ResourceDefinition,
	artifact ArtifactSpec,
) (artifactRequest, error) {
	downloadPath, err := artifact.downloadPath()
	if err != nil {
		return artifactRequest{}, err
	}
	downloadPath, err = cleanRelativePath(downloadPath, "download_path")
	if err != nil {
		return artifactRequest{}, err
	}

	resourcePath := ""
	if def.Extract {
		resourcePath, err = cleanOptionalRelativePath(artifact.ResourcePath, "resource_path")
		if err != nil {
			return artifactRequest{}, err
		}
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
		return artifactRequest{}, err
	}
	entryPath := filepath.Join(
		m.cache.artifactsRoot(),
		safePathSegment(resourceID),
		safePathSegment(platform),
		cacheKey,
	)

	target := artifactRequest{
		cacheKey:     cacheKey,
		platform:     platform,
		downloadPath: downloadPath,
		resourcePath: resourcePath,
		extract:      def.Extract,
	}

	return target.withEntryPath(m.cache, entryPath)
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
