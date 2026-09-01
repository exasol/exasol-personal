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

// resolution is where one descriptor's content lives. It is computed from the
// descriptor on every request rather than recorded, which is what lets two
// descriptors differing only in presentation share one cache entry.
type resolution struct {
	// key names both the index record and the entry directory.
	key string
	// identity is empty when nothing can identify the content, which forces a
	// refresh on every request.
	identity string
	// local is set when the source reported content already in place, in which
	// case the cache stores no copy of the artifact itself.
	local        string
	downloadPath string
	extract      bool
	subpath      string
	embedded     bool
}

func (*Manager) newResolution(
	def ResourceDefinition,
	artifact ArtifactSpec,
	probe Probe,
	identity string,
) (resolution, error) {
	downloadPath, err := cleanRelativePath(artifact.DownloadPath, "download_path")
	if err != nil {
		return resolution{}, err
	}
	if downloadPath == "" {
		downloadPath = urlBasename(artifact.URL)
	}

	subpath, err := cleanRelativePath(artifact.Subpath, "subpath")
	if err != nil {
		return resolution{}, err
	}
	if def.Glob {
		// A glob's subpath is a match pattern applied to the resolved result,
		// not a path to select within it.
		subpath = ""
	}

	// A glob resource's embedded bytes are always an archive of its matched
	// entries, even when its live source is a bare directory needing none.
	extract := def.Extract || (def.Glob && def.Embed != EmbedNever)

	return resolution{
		key:          cacheKeyFor(identity, artifact.URL, downloadPath),
		identity:     identity,
		local:        probe.Local,
		downloadPath: downloadPath,
		extract:      extract,
		subpath:      subpath,
		embedded:     def.Embed != EmbedNever,
	}, nil
}

// cacheKeyFor keys an entry on content identity alone, so the same content
// reached by two resources is stored once. Content that cannot be identified
// falls back to its location, which keeps unidentifiable resources from
// colliding with each other.
func cacheKeyFor(identity, url, downloadPath string) string {
	seed := identity
	if strings.TrimSpace(seed) == "" {
		seed = "url:" + url + "|" + downloadPath
	}
	sum := sha256.Sum256([]byte(seed))

	return hex.EncodeToString(sum[:])
}

// artifactPath is where the fetched artifact is stored, empty when the content
// stays where the source reported it.
func (m *Manager) artifactPath(res resolution) string {
	if res.local != "" {
		return ""
	}

	return filepath.Join(m.cache.entryDir(res.key), res.downloadPath)
}

// contentRoot is the directory or file a subpath is selected from.
func (m *Manager) contentRoot(res resolution) string {
	if res.extract {
		return filepath.Join(m.cache.entryDir(res.key), extractRelPath)
	}
	if res.local != "" {
		return res.local
	}

	return m.artifactPath(res)
}

func (m *Manager) resolvedPath(res resolution) (string, error) {
	root := m.contentRoot(res)
	if strings.TrimSpace(res.subpath) == "" {
		return root, nil
	}

	return pathWithinRoot(root, res.subpath, "subpath")
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

	// Sources take one URL string, so a ref declared as its own field is folded
	// back into it here.
	artifact.URL = artifact.Locator().String()
	artifact.Ref = ""

	// An embedded resource takes its identity from the build's own content hash
	// even when the artifact also declares a checksum: the checksum describes
	// what was fetched upstream, not what a particular build ended up
	// embedding, so only the build's hash tells two builds' content apart.
	//
	// A declared checksum already identifies the content, so a source is asked
	// to identify it only when none was declared. A local source is asked
	// regardless, since only it can report content that is already in place.
	var probe Probe
	locator := artifact.Locator()
	if def.Embed != EmbedNever {
		if hash, ok := lookupEmbeddedHash(resourceID); ok {
			artifact.Sha256 = hash
		}
	} else if strings.TrimSpace(artifact.Sha256) == "" || (FileSource{}).Handles(locator) {
		probe, err = probeSource(ctx, locator)
		if err != nil {
			return "", err
		}
	}

	identity := contentIdentity(artifact.Sha256, probe.Identity)
	res, err := m.newResolution(def, artifact, probe, identity)
	if err != nil {
		return "", err
	}

	// Content already in place that needs no unpacking is used where it is, so
	// the cache neither stores nor tracks it.
	if res.local != "" && !res.extract {
		return m.resolvedPath(res)
	}

	var resolvedPath string
	err = m.cache.withExclusiveLock(ctx, func() error {
		var lockErr error
		resolvedPath, lockErr = m.resolveUnderLock(ctx, resourceID, artifact, res)

		return lockErr
	})
	if err != nil {
		return "", err
	}

	return resolvedPath, nil
}

func (m *Manager) resolveUnderLock(
	ctx context.Context,
	resourceID string,
	artifact ArtifactSpec,
	res resolution,
) (string, error) {
	index, err := m.cache.readIndex()
	if err != nil {
		return "", err
	}

	if res.identity == "" {
		slog.Info(
			"re-fetching resource that cannot be identified, result may not be stable",
			"id", resourceID, "url", artifact.URL,
		)
	} else if path, ok := m.reuse(&index, resourceID, res); ok {
		slog.Debug("found resource in cache", "id", resourceID, "path", path)

		return path, nil
	}

	resolvedPath, err := m.materialize(ctx, resourceID, artifact, res, &index)
	if err != nil {
		return "", err
	}

	slog.Debug("fetched resource", "id", resourceID, "path", resolvedPath)

	return resolvedPath, nil
}

// reuse reports a cached path only when the index knows the identity and the
// bytes this descriptor needs are actually present, so a user who deleted part
// of the cache gets a refetch rather than a dangling path.
func (m *Manager) reuse(index *cacheIndex, resourceID string, res resolution) (string, bool) {
	entry, ok := index.Entries[res.key]
	if !ok {
		return "", false
	}

	needed := m.contentRoot(res)
	if _, err := os.Stat(needed); err != nil {
		return "", false
	}

	resolvedPath, err := m.resolvedPath(res)
	if err != nil {
		return "", false
	}
	if _, err := os.Stat(resolvedPath); err != nil {
		return "", false
	}

	entry.LastUsedAt = m.cache.clock().UTC()
	entry.ResourceIDs = withResourceID(entry.ResourceIDs, resourceID)
	if size, statErr := directorySize(m.cache.entryDir(res.key)); statErr == nil {
		entry.SizeBytes = size
	}
	index.Entries[res.key] = entry
	if err := m.writeIndexAfterCleanup(index); err != nil {
		return "", false
	}

	return resolvedPath, true
}

func (m *Manager) materialize(
	ctx context.Context,
	resourceID string,
	artifact ArtifactSpec,
	res resolution,
	index *cacheIndex,
) (string, error) {
	stagingDir, err := m.cache.newStagingDir()
	if err != nil {
		return "", err
	}
	defer func() { _ = os.RemoveAll(stagingDir) }()

	// content is what an extraction reads from: the staged artifact, or the
	// source's own copy when it reported the content already in place.
	content := res.local
	switch {
	case res.embedded:
		content = filepath.Join(stagingDir, res.downloadPath)
		if err := m.storeEmbedded(resourceID, content); err != nil {
			return "", err
		}
	case res.local != "":
		// The artifact stays where it is; only its extraction is cached.
	case res.extract:
		cachedArtifact := m.artifactPath(res)
		if _, recorded := index.Entries[res.key]; recorded {
			if _, statErr := os.Stat(cachedArtifact); statErr == nil {
				content = cachedArtifact
				break
			}
		}
		content = filepath.Join(stagingDir, res.downloadPath)
		if err := m.fetch(ctx, artifact, content); err != nil {
			return "", errors.Join(fmt.Errorf("failed to fetch resource %q", resourceID), err)
		}
	default:
		content = filepath.Join(stagingDir, res.downloadPath)
		if err := m.fetch(ctx, artifact, content); err != nil {
			return "", errors.Join(fmt.Errorf("failed to fetch resource %q", resourceID), err)
		}
	}

	if res.extract {
		if err := extractInto(content, filepath.Join(stagingDir, extractRelPath)); err != nil {
			return "", errors.Join(fmt.Errorf("failed to extract resource %q", resourceID), err)
		}
	}

	if err := m.cache.commitStaged(stagingDir, m.cache.entryDir(res.key)); err != nil {
		return "", err
	}

	size, sizeErr := directorySize(m.cache.entryDir(res.key))
	if sizeErr != nil {
		size = 0
	}

	now := m.cache.clock().UTC()
	entry := index.Entries[res.key]
	entry.ResourceIDs = withResourceID(entry.ResourceIDs, resourceID)
	entry.URL = artifact.URL
	entry.Identity = res.identity
	entry.Sha256 = normalizeSha256(artifact.Sha256)
	entry.DownloadPath = res.downloadPath
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = now
	}
	entry.LastUsedAt = now
	entry.SizeBytes = size
	index.Entries[res.key] = entry

	if err := m.writeIndexAfterCleanup(index); err != nil {
		return "", err
	}

	return m.resolvedPath(res)
}

func withResourceID(ids []string, resourceID string) []string {
	if slices.Contains(ids, resourceID) {
		return ids
	}
	ids = append(ids, resourceID)
	slices.Sort(ids)

	return ids
}

func (m *Manager) writeIndexAfterCleanup(index *cacheIndex) error {
	if err := m.cache.cleanupStaleIfDue(index); err != nil {
		slog.Warn("failed to clean resource cache; continuing", "error", err)
	}

	return m.cache.writeIndex(*index)
}

func (*Manager) fetch(ctx context.Context, artifact ArtifactSpec, fetchPath string) error {
	loc := artifact.Locator()
	source, err := sourceFor(loc)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(fetchPath), dirPerm); err != nil {
		return err
	}
	if err := source.Fetch(ctx, loc, fetchPath); err != nil {
		return err
	}

	return verifyStored(fetchPath, normalizeSha256(artifact.Sha256))
}

// storeEmbedded materializes a resource from data compiled into the binary. A
// registry miss is a hard error, never a fallback to the network sources list.
func (*Manager) storeEmbedded(resourceID, fetchPath string) error {
	data, ok := lookupEmbedded(resourceID)
	if !ok {
		return fmt.Errorf("no embedded data registered for resource %q", resourceID)
	}

	if err := os.MkdirAll(filepath.Dir(fetchPath), dirPerm); err != nil {
		return err
	}
	if err := os.WriteFile(fetchPath, data, filePerm); err != nil {
		return err
	}

	slog.Debug("resolved resource from embedded data", "id", resourceID)

	return nil
}

func extractInto(filename, extractPath string) error {
	for _, extractor := range extractors {
		if extractor.CanExtract(filename) {
			if err := os.MkdirAll(extractPath, dirPerm); err != nil {
				return err
			}

			return extractor.Extract(filename, extractPath)
		}
	}

	return fmt.Errorf("unsupported resource archive format in %q", filename)
}
