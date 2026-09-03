// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package resource

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

// Large downloads and extractions can hold the lock for several minutes.
const acquireTimeout = 5 * time.Minute

type cacheIndex struct {
	Version     int                        `json:"version"`
	LastCleanup time.Time                  `json:"lastCleanupAt,omitzero"`
	Entries     map[string]cacheIndexEntry `json:"entries"`
}

// Presentation is excluded because one content identity may have several views.
type cacheIndexEntry struct {
	// Content-addressed entries may be shared by several resource IDs.
	ResourceIDs  []string  `json:"resourceIds"`
	URL          string    `json:"url"`
	Identity     string    `json:"identity"`
	Sha256       string    `json:"sha256,omitempty"`
	DownloadPath string    `json:"downloadPath,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
	LastUsedAt   time.Time `json:"lastUsedAt"`
	SizeBytes    int64     `json:"sizeBytes"`
}

func (c *Cache) entryDir(key string) string {
	return filepath.Join(c.artifactsRoot(), key)
}

func (c *Cache) artifactFile(entryID string, entry cacheIndexEntry) string {
	if strings.TrimSpace(entry.DownloadPath) == "" {
		return ""
	}

	return filepath.Join(c.entryDir(entryID), entry.DownloadPath)
}

type integrityCheck struct {
	Status string
	Actual string
	Error  string
}

func (c *Cache) artifactsRoot() string {
	return filepath.Join(c.root, artifactsDirName)
}

func (c *Cache) stagingRoot() string {
	return filepath.Join(c.root, stagingDirName)
}

// Partial entries must stay outside committed cache paths.
func (c *Cache) newStagingDir() (string, error) {
	if err := os.MkdirAll(c.stagingRoot(), dirPerm); err != nil {
		return "", err
	}

	return os.MkdirTemp(c.stagingRoot(), "entry-")
}

// Renames keep each staged item atomic.
func (*Cache) commitStaged(stagingDir, entryDir string) error {
	if err := os.MkdirAll(filepath.Dir(entryDir), dirPerm); err != nil {
		return err
	}
	if _, err := os.Stat(entryDir); errors.Is(err, os.ErrNotExist) {
		return os.Rename(stagingDir, entryDir)
	}

	staged, err := os.ReadDir(stagingDir)
	if err != nil {
		return err
	}
	for _, item := range staged {
		target := filepath.Join(entryDir, item.Name())
		if err := os.RemoveAll(target); err != nil {
			return err
		}
		if err := os.Rename(filepath.Join(stagingDir, item.Name()), target); err != nil {
			return err
		}
	}

	return nil
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
	if errors.Is(err, directorymutex.ErrAcquireTimeout) {
		return ErrCacheLocked
	}

	return err
}

func emptyCacheIndex() cacheIndex {
	return cacheIndex{
		Version: cacheIndexVersion,
		Entries: map[string]cacheIndexEntry{},
	}
}

func (c *Cache) readIndex() (cacheIndex, error) {
	index, exists, err := c.readIndexRaw()
	if err != nil || !exists {
		return index, err
	}
	if err := c.validateIndex(index); err != nil {
		return index, err
	}

	return index, nil
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
	// Older indexes describe incompatible paths, but their data remains recoverable.
	if index.Version < cacheIndexVersion {
		return emptyCacheIndex(), true, nil
	}
	if index.Version > cacheIndexVersion {
		return index, true, fmt.Errorf(
			"resource cache was created by a newer version of Exasol Personal"+
				" (index version %d): upgrade Exasol Personal,"+
				" or run `exasol cache clean --all` to discard the cache",
			index.Version,
		)
	}
	if index.Entries == nil {
		index.Entries = map[string]cacheIndexEntry{}
	}

	return index, true, nil
}

func (c *Cache) SupersededIndexVersion() int {
	data, err := os.ReadFile(c.IndexPath())
	if err != nil {
		return 0
	}
	var header struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return 0
	}
	if header.Version == 0 || header.Version >= cacheIndexVersion {
		return 0
	}

	return header.Version
}

func (*Cache) validateIndex(index cacheIndex) error {
	for entryID, entry := range index.Entries {
		if _, err := cleanRelativePath(entry.DownloadPath, "downloadPath"); err != nil {
			return fmt.Errorf("cache entry %q: %w", entryID, err)
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

func verifyStored(path, declaredSha256 string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() || strings.TrimSpace(declaredSha256) == "" {
		return nil
	}

	actual, err := sha256OfFile(path)
	if err != nil {
		return err
	}
	if actual != declaredSha256 {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", declaredSha256, actual)
	}

	return nil
}

func (c *Cache) checkIntegrity(entryID string, entry cacheIndexEntry) integrityCheck {
	artifactPath := c.artifactFile(entryID, entry)
	if artifactPath == "" {
		return integrityCheck{Status: integrityStatusOK}
	}
	info, err := os.Stat(artifactPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return integrityCheck{Status: integrityStatusMissing, Error: err.Error()}
		}

		return integrityCheck{Status: integrityStatusReadError, Error: err.Error()}
	}

	if info.IsDir() || strings.TrimSpace(entry.Sha256) == "" {
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

func pathWithinRoot(root, value, field string) (string, error) {
	cleanValue, err := cleanRelativePath(value, field)
	if err != nil {
		return "", err
	}

	return filepath.Join(root, filepath.FromSlash(cleanValue)), nil
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
