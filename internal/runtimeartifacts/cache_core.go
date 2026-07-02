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
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/exasol/exasol-personal/internal/directorymutex"
)

// Primitive groups and semantics:
// - lock primitives (`withExclusiveLock`, `lockStatus`, `clearLock`) create the
//   cache root before constructing directorymutex, serialize cache reads and
//   mutations with exclusive locks, map contention to cache-specific errors,
//   report lock state without acquiring the lock, and release locks with a
//   cancellation-independent context.
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

// This timeout should be long enough for another launcher to download and
// extract whatever resource it is going to use.
const acquireTimeout = 5 * time.Minute

type cacheIndex struct {
	Version     int                        `json:"version"`
	LastCleanup time.Time                  `json:"lastCleanupAt,omitempty"`
	Entries     map[string]cacheIndexEntry `json:"entries"`
}

type cacheIndexEntry struct {
	Key          string    `json:"-"`
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
	RedirectPath string    `json:"redirectPath,omitempty"`
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

func (c *Cache) lockStatus() CacheLockStatus {
	status := CacheLockStatus{}
	info, err := os.Stat(c.root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return status
		}
		status.Error = err.Error()

		return status
	}
	status.CacheExists = true
	if !info.IsDir() {
		status.Error = c.root + " is not a directory"
		return status
	}

	mutex, err := directorymutex.New(c.root)
	if err != nil {
		status.Error = err.Error()
		return status
	}
	lockStatus, err := mutex.Status()
	if err != nil {
		status.Error = err.Error()
		return status
	}

	status.Locked = lockStatus.Locked
	status.Mode = lockStatus.Mode
	status.SharedCount = lockStatus.SharedCount
	status.MarkerPath = lockStatus.MarkerPath

	return status
}

//nolint:contextcheck // Lock release must outlive caller cancellation.
func (c *Cache) withExclusiveLock(ctx context.Context, function func() error) error {
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

	waitCtx, cancel := context.WithTimeout(ctx, acquireTimeout)
	defer cancel()

	err = mutex.WithExclusive(waitCtx, nil, func(any) error { return function() })
	if err != nil {
		return mapCacheLockError(err)
	}

	return nil
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
	info, err := os.Stat(artifactPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return integrityCheck{Status: integrityStatusMissing, Error: err.Error()}
		}

		return integrityCheck{Status: integrityStatusReadError, Error: err.Error()}
	}

	if info.IsDir() {
		return integrityCheck{Status: integrityStatusOK, Actual: entry.Sha256}
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

func cleanRelativePath(value, field string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", nil
	}

	cleanValue := filepath.Clean(filepath.FromSlash(value))
	if cleanValue == "." || cleanValue == ".." || filepath.IsAbs(cleanValue) ||
		strings.HasPrefix(cleanValue, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%s %q must stay within cache entry", field, value)
	}

	return filepath.ToSlash(cleanValue), nil
}

func sha256OfFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = file.Close()
	}()

	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}
