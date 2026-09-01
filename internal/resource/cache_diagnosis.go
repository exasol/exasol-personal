// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package resource

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
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
	now := c.clock()
	for _, entryID := range sortedEntryIDs(index) {
		expectedRoot := c.diagnoseEntry(&report, entryID, index.Entries[entryID], cfg, now)
		if expectedRoot != "" {
			expectedEntryRoots[expectedRoot] = struct{}{}
		}
	}
	report.UnexpectedPaths = c.unexpectedEntryRoots(expectedEntryRoots)

	return report
}

func (c *Cache) diagnoseEntry(
	report *DiagnosticReport,
	entryID string,
	entry cacheIndexEntry,
	cfg CacheConfig,
	now time.Time,
) string {
	info := c.entryInfo(entryID, entry)
	diag := DiagnosticEntry{CacheEntryInfo: info, ExpectedSha256: entry.Sha256}
	if _, err := cleanRelativePath(entry.DownloadPath, "downloadPath"); err != nil {
		diag.IntegrityStatus = integrityStatusReadError
		diag.Error = err.Error()
		report.InvalidArtifacts++
		report.Entries = append(report.Entries, diag)

		return ""
	}

	entryRoot := c.entryDir(entryID)
	relativeRoot, err := c.relativePath(entryRoot)
	if err != nil {
		relativeRoot = entryID
	}
	report.ArtifactCount++
	sizeBytes, _ := directorySize(entryRoot)
	report.TotalBytes += sizeBytes
	if isEntryStale(entry, cfg, now) {
		diag.Stale = true
		report.StaleCandidates++
	}
	check := c.checkIntegrity(entryID, entry)
	diag.IntegrityStatus = check.Status
	diag.ActualSha256 = check.Actual
	if check.Error != "" {
		diag.Error = check.Error
	}
	if check.Status != integrityStatusOK {
		report.InvalidArtifacts++
		if check.Status == integrityStatusMissing {
			report.MissingFiles = append(report.MissingFiles, c.artifactFile(entryID, entry))
		}
	}
	if _, err := os.Stat(entryRoot); errors.Is(err, os.ErrNotExist) {
		report.MissingFiles = append(report.MissingFiles, info.Path)
	}
	report.Entries = append(report.Entries, diag)

	return filepath.Clean(relativeRoot)
}

func (c *Cache) unexpectedEntryRoots(expected map[string]struct{}) []string {
	root := c.artifactsRoot()
	var unexpected []string
	entries, _ := os.ReadDir(root)
	for _, entry := range entries {
		pathValue := filepath.Join(root, entry.Name())
		cacheRel, err := c.relativePath(pathValue)
		if err != nil {
			continue
		}
		if _, ok := expected[filepath.Clean(cacheRel)]; !ok {
			unexpected = append(unexpected, filepath.ToSlash(cacheRel))
		}
	}
	slices.Sort(unexpected)
	if len(unexpected) > defaultUnexpectedReportSize {
		return unexpected[:defaultUnexpectedReportSize]
	}

	return unexpected
}
