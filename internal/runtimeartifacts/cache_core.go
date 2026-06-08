// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package runtimeartifacts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/exasol/exasol-personal/internal/directorymutex"
)

// Primitive groups and semantics:
// - lock primitives (`withExclusiveLock`, `clearLock`) create the cache root
//   before constructing directorymutex, serialize cache reads and mutations
//   with exclusive locks, map contention to cache-specific errors, and release
//   locks with a cancellation-independent context.
// - index primitives (`readIndex`, `readIndexRaw`, `writeIndex`) treat a
//   missing index as an empty cache, reject unsupported schema versions,
//   validate relative cache paths for normal operations, and write metadata
//   atomically through a temporary file.
// - path primitives (`pathWithinRoot`, `pathFromRelative`, `relativePath`,
//   `absolutePath`) normalize cache-relative paths and convert them to absolute
//   filesystem paths while rejecting values that escape their root.
// - integrity primitives (`checkIntegrity`) validate the cached downloaded
//   artifact against the expected checksum and do not inspect extracted
//   contents.
// - filesystem primitives (`directorySize`) provide size inspection without
//   mutating the cache.

type cacheIndex struct {
	Version     int                        `json:"version"`
	LastCleanup time.Time                  `json:"lastCleanupAt,omitempty"`
	Entries     map[string]cacheIndexEntry `json:"entries"`
}

type cacheIndexEntry struct {
	ResourceID   string    `json:"resourceId"`
	Platform     string    `json:"platform"`
	URL          string    `json:"url"`
	Sha256       string    `json:"sha256"`
	Extract      bool      `json:"extract"`
	DownloadPath string    `json:"downloadPath,omitempty"`
	ResourcePath string    `json:"resourcePath,omitempty"`
	EntryPath    string    `json:"entryPath"`
	ArtifactPath string    `json:"artifactPath"`
	ResolvedPath string    `json:"resolvedPath"`
	CreatedAt    time.Time `json:"createdAt"`
	LastUsedAt   time.Time `json:"lastUsedAt"`
	SizeBytes    int64     `json:"sizeBytes"`
}

type integrityCheck struct {
	Status string
	Actual string
	Error  string
}

type cacheLockedError struct {
	message string
}

func (e *cacheLockedError) Error() string {
	return e.message
}

func (*cacheLockedError) Is(target error) bool {
	return target == ErrCacheLocked
}

func (c *Cache) artifactsRoot() string {
	return filepath.Join(c.root, artifactsDirName)
}

func (c *Cache) downloadsRoot() string {
	return filepath.Join(c.root, downloadsDirName)
}

func (c *Cache) clearLock() error {
	if err := os.MkdirAll(c.root, dirPerm); err != nil {
		return err
	}
	mutex, err := directorymutex.New(c.root)
	if err != nil {
		return err
	}

	return mutex.ClearLock()
}

//nolint:contextcheck // Lock release must outlive caller cancellation.
func (c *Cache) withExclusiveLock(ctx context.Context, function func() error) (callbackErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := os.MkdirAll(c.root, dirPerm); err != nil {
		return err
	}
	mutex, err := directorymutex.New(c.root)
	if err != nil {
		return err
	}

	lockCtx, cancel := cacheLockContext(ctx)
	defer cancel()
	if err := mutex.AcquireExclusive(lockCtx); err != nil {
		return mapCacheLockError(err)
	}

	releaseCtx := context.WithoutCancel(ctx)
	defer func() {
		releaseErr := mutex.ReleaseExclusive(releaseCtx)
		if callbackErr == nil {
			callbackErr = releaseErr
		} else if releaseErr != nil {
			callbackErr = errors.Join(callbackErr, releaseErr)
		}
	}()

	callbackErr = function()

	return callbackErr
}

func cacheLockContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}

	return context.WithTimeout(ctx, defaultCacheAcquireTimeout)
}

func mapCacheLockError(err error) error {
	if errors.Is(err, context.Canceled) {
		return err
	}
	if errors.Is(err, directorymutex.ErrAcquireTimeout) {
		return &cacheLockedError{
			message: "Runtime artifact cache is locked by another operation. " +
				"Please wait. Run `exasol cache unlock` only if no launcher " +
				"process is using the cache.",
		}
	}

	return err
}

func emptyCacheIndex() cacheIndex {
	return cacheIndex{
		Version: cacheIndexVersion,
		Entries: map[string]cacheIndexEntry{},
	}
}

func (c *Cache) readIndex() (cacheIndex, bool, error) {
	index, exists, err := c.readIndexRaw()
	if err != nil || !exists {
		return index, exists, err
	}
	if err := c.validateIndex(index); err != nil {
		return index, true, err
	}

	return index, true, nil
}

func (c *Cache) readIndexRaw() (cacheIndex, bool, error) {
	index := emptyCacheIndex()
	data, err := os.ReadFile(c.IndexPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return index, false, nil
		}

		return index, false, err
	}
	if err := json.Unmarshal(data, &index); err != nil {
		return index, true, err
	}
	if index.Version == 0 {
		index.Version = cacheIndexVersion
	}
	if index.Version != cacheIndexVersion {
		return index, true, fmt.Errorf(
			"unsupported runtime artifact cache index version %d",
			index.Version,
		)
	}
	if index.Entries == nil {
		index.Entries = map[string]cacheIndexEntry{}
	}

	return index, true, nil
}

func (c *Cache) validateIndex(index cacheIndex) error {
	for entryID, entry := range index.Entries {
		for field, value := range map[string]string{
			"entryPath":    entry.EntryPath,
			"artifactPath": entry.ArtifactPath,
			"resolvedPath": entry.ResolvedPath,
		} {
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf("cache entry %q has empty %s", entryID, field)
			}
			if _, err := c.pathFromRelative(value, field); err != nil {
				return fmt.Errorf("cache entry %q: %w", entryID, err)
			}
		}
	}

	return nil
}

func (c *Cache) writeIndex(index cacheIndex) error {
	if index.Entries == nil {
		index.Entries = map[string]cacheIndexEntry{}
	}
	index.Version = cacheIndexVersion
	if err := os.MkdirAll(c.root, dirPerm); err != nil {
		return err
	}
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}

	tmpFile, err := os.CreateTemp(c.root, cacheIndexFileName+".tmp-")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	closed := false
	removeTemp := true
	defer func() {
		if !closed {
			_ = tmpFile.Close()
		}
		if removeTemp {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := tmpFile.Write(data); err != nil {
		return err
	}
	if err := tmpFile.Chmod(filePerm); err != nil {
		return err
	}
	if err := tmpFile.Sync(); err != nil {
		return err
	}
	if err := tmpFile.Close(); err != nil {
		closed = true
		return err
	}
	closed = true

	if err := os.Rename(tmpPath, c.IndexPath()); err != nil {
		return err
	}
	removeTemp = false

	return nil
}

func (c *Cache) checkIntegrity(entry cacheIndexEntry) integrityCheck {
	artifactPath, err := c.pathFromRelative(entry.ArtifactPath, "artifactPath")
	if err != nil {
		return integrityCheck{Status: integrityStatusReadError, Error: err.Error()}
	}
	if _, err := os.Stat(artifactPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return integrityCheck{Status: integrityStatusMissing, Error: err.Error()}
		}

		return integrityCheck{Status: integrityStatusReadError, Error: err.Error()}
	}
	actual, err := sha256OfFile(artifactPath)
	if err != nil {
		return integrityCheck{Status: integrityStatusReadError, Error: err.Error()}
	}
	if actual != entry.Sha256 {
		return integrityCheck{Status: integrityStatusMismatch, Actual: actual}
	}

	return integrityCheck{Status: integrityStatusOK, Actual: actual}
}

func (c *Cache) pathFromRelative(value, field string) (string, error) {
	return pathWithinRoot(c.root, value, field)
}

func (c *Cache) relativePath(pathValue string) (string, error) {
	rel, err := filepath.Rel(c.root, pathValue)
	if err != nil {
		return "", err
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q must stay within %s", pathValue, c.root)
	}

	return filepath.ToSlash(rel), nil
}

func (c *Cache) absolutePath(rel string) string {
	return filepath.Join(c.root, filepath.FromSlash(rel))
}

func pathWithinRoot(root, value, field string) (string, error) {
	cleanValue, err := cleanRelativePath(value, field)
	if err != nil {
		return "", err
	}

	candidate := filepath.Join(root, filepath.FromSlash(cleanValue))
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%s %q must stay within %s", field, value, root)
	}

	return candidate, nil
}

func cleanOptionalRelativePath(value, field string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", nil
	}

	return cleanRelativePath(value, field)
}

func cleanRelativePath(value, field string) (string, error) {
	cleanValue := filepath.Clean(filepath.FromSlash(value))
	if cleanValue == "." || cleanValue == ".." || filepath.IsAbs(cleanValue) ||
		strings.HasPrefix(cleanValue, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%s %q must stay within cache entry", field, value)
	}

	return filepath.ToSlash(cleanValue), nil
}

func safePathSegment(value string) string {
	var builder strings.Builder
	for _, char := range value {
		switch {
		case char >= 'a' && char <= 'z',
			char >= 'A' && char <= 'Z',
			char >= '0' && char <= '9',
			char == '-', char == '_', char == '.':
			_, _ = builder.WriteRune(char)
		default:
			_ = builder.WriteByte('_')
		}
	}
	if builder.Len() == 0 {
		return "_"
	}

	return builder.String()
}

func directorySize(path string) (int64, error) {
	var size int64
	err := filepath.WalkDir(path, func(_ string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		size += info.Size()

		return nil
	})

	return size, err
}
