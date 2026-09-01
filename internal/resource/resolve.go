// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package resource

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Presentation is not part of cache identity.
type resolution struct {
	key          string
	identity     string
	local        string
	downloadPath string
	extract      bool
	subpath      string
}

func (*Resolver) newResolution(
	descriptor Descriptor,
	probe Probe,
	identity string,
) (resolution, error) {
	downloadPath, err := cleanRelativePath(descriptor.DownloadPath, "download_path")
	if err != nil {
		return resolution{}, err
	}
	if downloadPath == "" {
		downloadPath = urlBasename(descriptor.Locator.URL)
	}

	subpath, err := cleanRelativePath(descriptor.Subpath, "subpath")
	if err != nil {
		return resolution{}, err
	}

	return resolution{
		key:          cacheKeyFor(identity, descriptor.Locator, downloadPath),
		identity:     identity,
		local:        probe.Local,
		downloadPath: downloadPath,
		extract:      descriptor.Extract,
		subpath:      subpath,
	}, nil
}

// Unidentified resources use their location to avoid collisions.
func cacheKeyFor(identity string, loc Locator, downloadPath string) string {
	seed := identity
	if strings.TrimSpace(seed) == "" {
		seed = "url:" + loc.String() + "|" + downloadPath
	}
	sum := sha256.Sum256([]byte(seed))

	return hex.EncodeToString(sum[:])
}

func (r *Resolver) artifactPath(res resolution) string {
	if res.local != "" {
		return ""
	}

	return filepath.Join(r.cache.entryDir(res.key), res.downloadPath)
}

func (r *Resolver) contentRoot(res resolution) string {
	if res.extract {
		return filepath.Join(r.cache.entryDir(res.key), extractRelPath)
	}
	if res.local != "" {
		return res.local
	}

	return r.artifactPath(res)
}

func (r *Resolver) resolvedPath(res resolution) (string, error) {
	root := r.contentRoot(res)
	if strings.TrimSpace(res.subpath) == "" {
		return root, nil
	}

	return pathWithinRoot(root, res.subpath, "subpath")
}

func (r *Resolver) resolveArtifact(
	ctx context.Context,
	descriptor Descriptor,
	resourceID string,
) (string, error) {
	// Only a local probe can tell the resolver to bypass artifact storage.
	var (
		probe Probe
		err   error
	)
	if strings.TrimSpace(descriptor.Sha256) == "" ||
		(FileSource{}).Handles(descriptor.Locator) {
		probe, err = r.probeSource(ctx, descriptor.Locator)
		if err != nil {
			return "", err
		}
	}

	identity := contentIdentity(descriptor.Sha256, probe.Identity)
	res, err := r.newResolution(descriptor, probe, identity)
	if err != nil {
		return "", err
	}

	if res.local != "" && !res.extract {
		return r.resolvedPath(res)
	}

	var resolvedPath string
	err = r.cache.withExclusiveLock(ctx, func() error {
		var lockErr error
		resolvedPath, lockErr = r.resolveUnderLock(ctx, resourceID, descriptor, res)

		return lockErr
	})
	if err != nil {
		return "", err
	}

	return resolvedPath, nil
}

func (r *Resolver) resolveUnderLock(
	ctx context.Context,
	resourceID string,
	descriptor Descriptor,
	res resolution,
) (string, error) {
	index, err := r.cache.readIndex()
	if err != nil {
		return "", err
	}

	if res.identity == "" {
		slog.Info(
			"re-fetching resource that cannot be identified, result may not be stable",
			"id", resourceID, "url", descriptor.Locator.String(),
		)
	} else if path, ok := r.reuse(&index, resourceID, res); ok {
		slog.Debug("found resource in cache", "id", resourceID, "path", path)

		return path, nil
	}

	resolvedPath, err := r.materialize(ctx, resourceID, descriptor, res, &index)
	if err != nil {
		return "", err
	}

	slog.Debug("fetched resource", "id", resourceID, "path", resolvedPath)

	return resolvedPath, nil
}

// Missing cache files force a refetch.
func (r *Resolver) reuse(index *cacheIndex, resourceID string, res resolution) (string, bool) {
	entry, ok := index.Entries[res.key]
	if !ok {
		return "", false
	}

	needed := r.contentRoot(res)
	if _, err := os.Stat(needed); err != nil {
		return "", false
	}

	resolvedPath, err := r.resolvedPath(res)
	if err != nil {
		return "", false
	}
	if _, err := os.Stat(resolvedPath); err != nil {
		return "", false
	}

	entry.LastUsedAt = r.cache.clock().UTC()
	entry.ResourceIDs = withResourceID(entry.ResourceIDs, resourceID)
	if size, statErr := directorySize(r.cache.entryDir(res.key)); statErr == nil {
		entry.SizeBytes = size
	}
	index.Entries[res.key] = entry
	if err := r.writeIndexAfterCleanup(index); err != nil {
		return "", false
	}

	return resolvedPath, true
}

func (r *Resolver) materialize(
	ctx context.Context,
	resourceID string,
	descriptor Descriptor,
	res resolution,
	index *cacheIndex,
) (string, error) {
	stagingDir, err := r.cache.newStagingDir()
	if err != nil {
		return "", err
	}
	defer func() { _ = os.RemoveAll(stagingDir) }()

	content := res.local
	needsFetch := res.local == ""
	if needsFetch && res.extract {
		cachedArtifact := r.artifactPath(res)
		if _, recorded := index.Entries[res.key]; recorded {
			if _, statErr := os.Stat(cachedArtifact); statErr == nil {
				content = cachedArtifact
				needsFetch = false
			}
		}
	}
	if needsFetch {
		content = filepath.Join(stagingDir, res.downloadPath)
		if err := r.fetch(ctx, descriptor, content); err != nil {
			return "", errors.Join(fmt.Errorf("failed to fetch resource %q", resourceID), err)
		}
	}

	if res.extract {
		if err := r.extractInto(content, filepath.Join(stagingDir, extractRelPath)); err != nil {
			return "", errors.Join(fmt.Errorf("failed to extract resource %q", resourceID), err)
		}
	}

	if err := r.cache.commitStaged(stagingDir, r.cache.entryDir(res.key)); err != nil {
		return "", err
	}

	size, sizeErr := directorySize(r.cache.entryDir(res.key))
	if sizeErr != nil {
		size = 0
	}

	now := r.cache.clock().UTC()
	entry := index.Entries[res.key]
	entry.ResourceIDs = withResourceID(entry.ResourceIDs, resourceID)
	entry.URL = descriptor.Locator.String()
	entry.Identity = res.identity
	entry.Sha256 = normalizeSha256(descriptor.Sha256)
	entry.DownloadPath = res.downloadPath
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = now
	}
	entry.LastUsedAt = now
	entry.SizeBytes = size
	index.Entries[res.key] = entry

	if err := r.writeIndexAfterCleanup(index); err != nil {
		return "", err
	}

	return r.resolvedPath(res)
}

func withResourceID(ids []string, resourceID string) []string {
	if slices.Contains(ids, resourceID) {
		return ids
	}
	ids = append(ids, resourceID)
	slices.Sort(ids)

	return ids
}

func (r *Resolver) writeIndexAfterCleanup(index *cacheIndex) error {
	if err := r.cache.cleanupStaleIfDue(index); err != nil {
		slog.Warn("failed to clean resource cache; continuing", "error", err)
	}

	return r.cache.writeIndex(*index)
}

func (r *Resolver) fetch(ctx context.Context, descriptor Descriptor, fetchPath string) error {
	source, err := r.sourceFor(descriptor.Locator)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(fetchPath), dirPerm); err != nil {
		return err
	}
	if err := source.Fetch(ctx, descriptor.Locator, fetchPath); err != nil {
		return err
	}

	return verifyStored(fetchPath, normalizeSha256(descriptor.Sha256))
}

func (r *Resolver) extractInto(filename, extractPath string) error {
	for _, extractor := range r.extractors {
		if extractor.CanExtract(filename) {
			if err := os.MkdirAll(extractPath, dirPerm); err != nil {
				return err
			}

			return extractor.Extract(filename, extractPath)
		}
	}

	return fmt.Errorf("unsupported resource archive format in %q", filename)
}
