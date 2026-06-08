// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package runtimeartifacts

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const defaultUnexpectedReportSize = 50

type DiagnosticReport struct {
	CacheRoot        string            `json:"cacheRoot"`
	ConfigPath       string            `json:"configPath"`
	ConfigExists     bool              `json:"configExists"`
	ConfigError      string            `json:"configError,omitempty"`
	RetentionDays    int               `json:"retentionDays"`
	IndexPath        string            `json:"indexPath"`
	IndexExists      bool              `json:"indexExists"`
	IndexError       string            `json:"indexError,omitempty"`
	Lock             CacheLockStatus   `json:"lock"`
	ArtifactCount    int               `json:"artifactCount"`
	TotalBytes       int64             `json:"totalBytes"`
	StaleCandidates  int               `json:"staleCandidates"`
	InvalidArtifacts int               `json:"invalidArtifacts"`
	MissingFiles     []string          `json:"missingFiles,omitempty"`
	UnexpectedPaths  []string          `json:"unexpectedPaths,omitempty"`
	Entries          []DiagnosticEntry `json:"entries"`
}

type DiagnosticEntry struct {
	CacheEntryInfo

	Stale           bool   `json:"stale"`
	IntegrityStatus string `json:"integrityStatus"`
	ExpectedSha256  string `json:"expectedSha256,omitempty"`
	ActualSha256    string `json:"actualSha256,omitempty"`
	Error           string `json:"error,omitempty"`
}

type diagnosticEntryResult struct {
	entry             DiagnosticEntry
	expectedEntryRoot string
	sizeBytes         int64
	counted           bool
	stale             bool
	invalid           bool
	missingFiles      []string
}

func (c *Cache) Diagnose() DiagnosticReport {
	report := DiagnosticReport{
		CacheRoot:  c.root,
		ConfigPath: c.configPath,
		IndexPath:  c.IndexPath(),
		Lock:       c.lockStatus(),
	}

	cfg, configExists, configErr := LoadCacheConfig(c.configPath)
	report.ConfigExists = configExists
	report.RetentionDays = cfg.RetentionDays
	if configErr != nil {
		report.ConfigError = configErr.Error()
		cfg = DefaultCacheConfig()
	}

	index, indexExists, indexErr := c.readIndexRaw()
	report.IndexExists = indexExists
	if indexErr != nil {
		report.IndexError = indexErr.Error()
		return report
	}

	expectedEntryRoots := map[string]struct{}{}
	now := c.clock.Now()
	for _, entryID := range sortedEntryIDs(index) {
		entry := index.Entries[entryID]
		result := c.diagnoseEntry(entryID, entry, cfg, now)
		if result.expectedEntryRoot != "" {
			expectedEntryRoots[result.expectedEntryRoot] = struct{}{}
		}
		if result.counted {
			report.ArtifactCount++
		}
		report.TotalBytes += result.sizeBytes
		if result.stale {
			report.StaleCandidates++
		}
		if result.invalid {
			report.InvalidArtifacts++
		}
		report.MissingFiles = append(report.MissingFiles, result.missingFiles...)
		report.Entries = append(report.Entries, result.entry)
	}
	report.UnexpectedPaths = c.unexpectedEntryRoots(expectedEntryRoots)

	return report
}

func (c *Cache) diagnoseEntry(
	entryID string,
	entry cacheIndexEntry,
	cfg CacheConfig,
	now time.Time,
) diagnosticEntryResult {
	info, err := c.entryInfo(entryID, entry)
	diag := DiagnosticEntry{CacheEntryInfo: info, ExpectedSha256: entry.Sha256}
	if err != nil {
		diag.ID = entryID
		diag.ResourceID = entry.ResourceID
		diag.Platform = entry.Platform
		diag.IntegrityStatus = integrityStatusReadError
		diag.Error = err.Error()

		return diagnosticEntryResult{entry: diag, invalid: true}
	}

	result := diagnosticEntryResult{
		entry:             diag,
		expectedEntryRoot: filepath.Clean(entry.EntryPath),
		counted:           true,
	}
	result.sizeBytes, _ = directorySize(c.absolutePath(entry.EntryPath))
	if isEntryStale(entry, cfg, now) {
		result.entry.Stale = true
		result.stale = true
	}
	check := c.checkIntegrity(entry)
	result.entry.IntegrityStatus = check.Status
	result.entry.ActualSha256 = check.Actual
	if check.Error != "" {
		result.entry.Error = check.Error
	}
	if check.Status != integrityStatusOK {
		result.invalid = true
		if check.Status == integrityStatusMissing {
			result.missingFiles = append(result.missingFiles, info.ArtifactPath)
		}
	}
	if entry.ResolvedPath != "" {
		if _, err := os.Stat(c.absolutePath(entry.ResolvedPath)); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				result.missingFiles = append(result.missingFiles, info.ResolvedPath)
			}
		}
	}

	return result
}

func (c *Cache) unexpectedEntryRoots(expected map[string]struct{}) []string {
	root := c.artifactsRoot()
	var unexpected []string
	_ = filepath.WalkDir(root, func(pathValue string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() || pathValue == root {
			return nil
		}
		rel, err := filepath.Rel(root, pathValue)
		if err != nil {
			return err
		}
		const cacheEntryPathDepth = 3
		if len(strings.Split(filepath.ToSlash(rel), "/")) != cacheEntryPathDepth {
			return nil
		}
		cacheRel, err := c.relativePath(pathValue)
		if err != nil {
			return err
		}
		if _, ok := expected[filepath.Clean(cacheRel)]; !ok {
			unexpected = append(unexpected, filepath.ToSlash(cacheRel))
		}

		return filepath.SkipDir
	})
	sort.Strings(unexpected)
	if len(unexpected) > defaultUnexpectedReportSize {
		return unexpected[:defaultUnexpectedReportSize]
	}

	return unexpected
}
