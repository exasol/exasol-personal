// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package runtimeartifacts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	resourceDirName = "resources"
	cacheFileName   = "resources.json"
	dirPerm         = 0o700
	filePerm        = 0o600
)

type Platform struct {
	GOOS   string
	GOARCH string
}

type Manager struct {
	spec        ResourceSpec
	resourceDir string
	cachePath   string
	platform    Platform
}

type cacheFile struct {
	Resources map[string]cacheEntry `json:"resources"`
}

type cacheEntry struct {
	URL    string `json:"url"`
	Sha256 string `json:"sha256"`
}

type artifactRequest struct {
	artifactPath string
	resolvedPath string
	extract      bool
}

func NewResourceManager(spec ResourceSpec, deploymentDir string) *Manager {
	return NewResourceManagerForPlatform(spec, deploymentDir, runtime.GOOS, runtime.GOARCH)
}

func NewResourceManagerForPlatform(spec ResourceSpec, deploymentDir, goos, goarch string) *Manager {
	return &Manager{
		spec:        spec,
		resourceDir: filepath.Join(deploymentDir, resourceDirName),
		cachePath:   filepath.Join(deploymentDir, cacheFileName),
		platform: Platform{
			GOOS:   goos,
			GOARCH: goarch,
		},
	}
}

func (m *Manager) Request(ctx context.Context, resourceID string) (string, error) {
	def, ok := m.spec[resourceID]
	if !ok {
		return "", fmt.Errorf("unknown runtime resource %q", resourceID)
	}

	artifact, err := def.Resolve(m.platform.GOOS, m.platform.GOARCH)
	if err != nil {
		return "", err
	}
	target, err := m.resolveArtifactRequest(resourceID, def, artifact)
	if err != nil {
		return "", err
	}

	cache, err := m.readCache()
	if err != nil {
		return "", err
	}

	if entry, ok := cache.Resources[resourceID]; ok {
		if resolvedPath, err := validateCacheEntry(entry, artifact, target); err == nil {
			return resolvedPath, nil
		}
	}

	return m.refresh(ctx, resourceID, artifact, target, cache)
}

func (m *Manager) resourcePath(resourceID string) string {
	return filepath.Join(m.resourceDir, resourceID)
}

func (m *Manager) readCache() (cacheFile, error) {
	cache := cacheFile{Resources: map[string]cacheEntry{}}

	data, err := os.ReadFile(m.cachePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cache, nil
		}

		return cache, err
	}

	if err := json.Unmarshal(data, &cache); err != nil {
		return cache, err
	}
	if cache.Resources == nil {
		cache.Resources = map[string]cacheEntry{}
	}

	return cache, nil
}

func (m *Manager) writeCache(cache cacheFile) error {
	if err := os.MkdirAll(filepath.Dir(m.cachePath), dirPerm); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := m.cachePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, filePerm); err != nil {
		return err
	}

	_ = os.Remove(m.cachePath)

	return os.Rename(tmpPath, m.cachePath)
}

func validateCacheEntry(
	entry cacheEntry,
	artifact ArtifactSpec,
	target artifactRequest,
) (string, error) {
	artifactPath := target.artifactPath
	resolvedPath := target.resolvedPath
	extract := target.extract

	if entry.URL != artifact.URL {
		return "", errors.New("url changed")
	}
	if entry.Sha256 != strings.ToLower(strings.TrimSpace(artifact.Sha256)) {
		return "", errors.New("checksum changed")
	}

	if _, err := os.Stat(artifactPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", errors.New("artifact missing")
		}

		return "", err
	}

	if !extract {
		return artifactPath, nil
	}

	if _, err := os.Stat(resolvedPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", errors.New("extracted output missing")
		}

		return "", err
	}

	return resolvedPath, nil
}

func (m *Manager) refresh(
	ctx context.Context,
	resourceID string,
	artifact ArtifactSpec,
	target artifactRequest,
	cache cacheFile,
) (string, error) {
	resourceDir := m.resourcePath(resourceID)
	if err := os.RemoveAll(resourceDir); err != nil {
		return "", err
	}
	if err := downloadArtifact(
		ctx,
		resourceID,
		artifact.URL,
		target.artifactPath,
		artifact.Sha256,
	); err != nil {
		return "", err
	}

	if target.extract {
		extractedPath, err := extractArtifact(target.artifactPath, artifact.ResourcePath)
		if err != nil {
			return "", err
		}
		target.resolvedPath = extractedPath
	}

	expected := strings.ToLower(strings.TrimSpace(artifact.Sha256))
	entry := cacheEntry{
		URL:    artifact.URL,
		Sha256: expected,
	}
	cache.Resources[resourceID] = entry
	if err := m.writeCache(cache); err != nil {
		return "", err
	}

	if target.extract {
		return target.resolvedPath, nil
	}

	return target.artifactPath, nil
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

	resourceDir := m.resourcePath(resourceID)
	downloadPath, err = pathWithinRoot(resourceDir, downloadPath, "download_path")
	if err != nil {
		return artifactRequest{}, err
	}
	target := artifactRequest{artifactPath: downloadPath, resolvedPath: downloadPath}
	if def.Extract {
		extractedRoot, err := archiveExtractionRoot(target.artifactPath)
		if err != nil {
			return artifactRequest{}, err
		}
		target.resolvedPath, err = extractedResourcePath(extractedRoot, artifact.ResourcePath)
		if err != nil {
			return artifactRequest{}, err
		}
		target.extract = true
	}

	return target, nil
}

func extractedResourcePath(extractedRoot, resourcePath string) (string, error) {
	if strings.TrimSpace(resourcePath) == "" {
		return extractedRoot, nil
	}

	return pathWithinRoot(extractedRoot, resourcePath, "resource_path")
}

func pathWithinRoot(root, value, field string) (string, error) {
	cleanValue := filepath.Clean(filepath.FromSlash(value))
	if cleanValue == "." || cleanValue == ".." || filepath.IsAbs(cleanValue) {
		return "", fmt.Errorf("%s %q must stay within %s", field, value, root)
	}

	candidate := filepath.Join(root, cleanValue)
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%s %q must stay within %s", field, value, root)
	}

	return candidate, nil
}
