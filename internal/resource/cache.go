// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package resource

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/exasol/exasol-personal/internal/directorymutex"
	"github.com/exasol/exasol-personal/internal/launcherpaths"
	"go.yaml.in/yaml/v3"
)

const (
	resourcesDirName         = "resources"
	artifactsDirName         = "artifacts"
	downloadsDirName         = "downloads"
	cacheIndexFileName       = "index.json"
	cacheConfigFileName      = "resources.yaml"
	cacheIndexVersion        = 2
	defaultRetentionDays     = 30
	automaticCleanupInterval = 24 * time.Hour
	integrityStatusOK        = "ok"
	integrityStatusMissing   = "missing"
	integrityStatusMismatch  = "mismatch"
	integrityStatusReadError = "read_error"
)

var (
	ErrInvalidCacheConfig = errors.New("invalid resource cache configuration")
	ErrCacheLocked        = errors.New(
		"resource cache is locked by another operation; please wait; " +
			"run `exasol cache unlock` only if no launcher process is using the cache",
	)
)

type CacheConfig struct {
	//nolint:tagliatelle // YAML config uses snake_case field names.
	RetentionDays int `json:"retentionDays" yaml:"retention_days"`
}

type Cache struct {
	root       string
	configPath string
	clock      func() time.Time
}

type CacheEntryInfo struct {
	ID string `json:"id"`
	// ResourceIDs lists every resource sharing this entry, normally just one.
	ResourceIDs []string  `json:"resourceIds"`
	URL         string    `json:"url"`
	Identity    string    `json:"identity"`
	Sha256      string    `json:"sha256,omitempty"`
	Path        string    `json:"path"`
	CreatedAt   time.Time `json:"createdAt"`
	LastUsedAt  time.Time `json:"lastUsedAt"`
	SizeBytes   int64     `json:"sizeBytes"`
}

type CleanupMode string

const (
	CleanupModeStale            CleanupMode = "stale"
	CleanupModeInvalid          CleanupMode = "invalid"
	CleanupModeAll              CleanupMode = "all"
	CleanupModePartialDownloads CleanupMode = "partial-downloads"
)

type CleanOptions struct {
	Mode   CleanupMode
	DryRun bool
}

type CleanSummary struct {
	Mode           CleanupMode      `json:"mode"`
	DryRun         bool             `json:"dryRun"`
	RemovedEntries int              `json:"removedEntries"`
	RemovedBytes   int64            `json:"removedBytes"`
	InvalidEntries int              `json:"invalidEntries,omitempty"`
	Entries        []CacheEntryInfo `json:"entries"`
}

type CacheLockStatus struct {
	CacheExists bool   `json:"cacheExists"`
	Locked      bool   `json:"locked"`
	Mode        string `json:"mode,omitempty"`
	SharedCount int    `json:"sharedCount,omitempty"`
	MarkerPath  string `json:"markerPath,omitempty"`
	Error       string `json:"error,omitempty"`
}

type partialDownloadPlan struct {
	removedBytes int64
	candidates   []partialDownloadCandidate
}

type partialDownloadCandidate struct {
	path string
}

func DefaultCacheRoot() (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve user cache directory: %w", err)
	}

	return filepath.Join(launcherpaths.DirPath(cacheDir), resourcesDirName), nil
}

func DefaultConfigPath() (string, error) {
	rootDir, err := launcherpaths.RootDirPath()
	if err != nil {
		return "", err
	}

	return filepath.Join(rootDir, cacheConfigFileName), nil
}

func NewDefaultCache() (*Cache, error) {
	root, err := DefaultCacheRoot()
	if err != nil {
		return nil, err
	}
	configPath, err := DefaultConfigPath()
	if err != nil {
		return nil, err
	}

	return NewCache(root, configPath), nil
}

func NewCache(root, configPath string) *Cache {
	return newCacheWithClock(root, configPath, time.Now)
}

func newCacheWithClock(root, configPath string, clock func() time.Time) *Cache {
	return &Cache{root: filepath.Clean(root), configPath: filepath.Clean(configPath), clock: clock}
}

func (c *Cache) Root() string {
	return c.root
}

func (c *Cache) IndexPath() string {
	return filepath.Join(c.root, cacheIndexFileName)
}

func DefaultCacheConfig() CacheConfig {
	return CacheConfig{RetentionDays: defaultRetentionDays}
}

func LoadCacheConfig(path string) (CacheConfig, bool, error) {
	cfg := DefaultCacheConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, false, nil
		}

		return cfg, false, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, true, err
	}
	if err := validateCacheConfig(cfg); err != nil {
		return cfg, true, err
	}

	return cfg, true, nil
}

func EnsureCacheConfig(path string) (CacheConfig, error) {
	cfg, exists, err := LoadCacheConfig(path)
	if err != nil || exists {
		return cfg, err
	}

	if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
		return cfg, err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return cfg, err
	}

	return cfg, os.WriteFile(path, data, filePerm)
}

func validateCacheConfig(cfg CacheConfig) error {
	if cfg.RetentionDays <= 0 {
		return fmt.Errorf("%w: retention_days must be positive", ErrInvalidCacheConfig)
	}

	return nil
}

func (c *Cache) List(ctx context.Context) ([]CacheEntryInfo, error) {
	var entries []CacheEntryInfo
	err := c.withExclusiveLock(ctx, func() error {
		index, err := c.readIndex()
		if err != nil {
			return err
		}
		entries = c.listEntries(index)

		return nil
	})

	return entries, err
}

func (c *Cache) Clean(ctx context.Context, opts CleanOptions) (CleanSummary, error) {
	mode, err := normalizeCleanupMode(opts.Mode)
	if err != nil {
		return CleanSummary{}, err
	}
	opts.Mode = mode

	var summary CleanSummary
	err = c.withExclusiveLock(ctx, func() error {
		cfg, err := EnsureCacheConfig(c.configPath)
		if err != nil {
			return err
		}
		if opts.Mode == CleanupModePartialDownloads {
			summary, err = c.cleanPartialDownloads(opts.DryRun)

			return err
		}

		summary, err = c.cleanIndexedEntries(cfg, opts)

		return err
	})

	return summary, err
}

func (c *Cache) Unlock() error {
	return c.clearLock()
}

//nolint:revive // dryRun mirrors the command-level --dry-run flag.
func (c *Cache) cleanPartialDownloads(dryRun bool) (CleanSummary, error) {
	plan, err := c.planPartialDownloadCleanup()
	summary := plan.summary(dryRun)
	if err != nil || dryRun {
		return summary, err
	}

	return summary, removePartialDownloads(plan)
}

func (c *Cache) cleanIndexedEntries(cfg CacheConfig, opts CleanOptions) (CleanSummary, error) {
	index, err := c.readIndex()
	if err != nil {
		// If we can't read the cache index, we treat it as empty so we can
		// still wipe the cache.
		index = emptyCacheIndex()
	}
	summary := c.planCleanup(index, cfg, opts)
	if !opts.DryRun {
		if opts.Mode == CleanupModeAll {
			err = c.wipeCacheContents(&index)
		} else {
			err = c.removeEntries(&index, summary)
		}
	}
	if err != nil || opts.DryRun {
		return summary, err
	}
	index.LastCleanup = c.clock().UTC()

	return summary, c.writeIndex(index)
}

func (c *Cache) listEntries(index cacheIndex) []CacheEntryInfo {
	entries := make([]CacheEntryInfo, 0, len(index.Entries))
	for _, entryID := range sortedEntryIDs(index) {
		entries = append(entries, c.entryInfo(entryID, index.Entries[entryID]))
	}

	return entries
}

func (c *Cache) entryInfo(entryID string, entry cacheIndexEntry) CacheEntryInfo {
	return CacheEntryInfo{
		ID:          entryID,
		ResourceIDs: slices.Clone(entry.ResourceIDs),
		URL:         entry.URL,
		Identity:    entry.Identity,
		Sha256:      entry.Sha256,
		Path:        c.entryDir(entryID),
		CreatedAt:   entry.CreatedAt,
		LastUsedAt:  entry.LastUsedAt,
		SizeBytes:   entry.SizeBytes,
	}
}

func (c *Cache) planCleanup(
	index cacheIndex,
	cfg CacheConfig,
	opts CleanOptions,
) CleanSummary {
	summary := CleanSummary{Mode: opts.Mode, DryRun: opts.DryRun}
	now := c.clock()
	for _, entryID := range sortedEntryIDs(index) {
		entry := index.Entries[entryID]
		invalid := false
		var remove bool
		switch opts.Mode {
		case CleanupModeAll:
			remove = true
		case CleanupModeStale:
			remove = isEntryStale(entry, cfg, now)
		case CleanupModeInvalid:
			check := c.checkIntegrity(entryID, entry)
			invalid = check.Status != integrityStatusOK
			remove = invalid
		case CleanupModePartialDownloads:
			remove = false
		default:
			remove = isEntryStale(entry, cfg, now)
		}
		if !remove {
			continue
		}
		info := c.entryInfo(entryID, entry)
		size, err := directorySize(c.entryDir(entryID))
		if err == nil {
			info.SizeBytes = size
		}
		summary.RemovedBytes += info.SizeBytes
		if invalid {
			summary.InvalidEntries++
		}
		summary.Entries = append(summary.Entries, info)
	}
	summary.RemovedEntries = len(summary.Entries)

	return summary
}

func normalizeCleanupMode(mode CleanupMode) (CleanupMode, error) {
	if mode == "" {
		return CleanupModeStale, nil
	}
	switch mode {
	case CleanupModeStale, CleanupModeInvalid, CleanupModeAll, CleanupModePartialDownloads:
		return mode, nil
	default:
		return "", fmt.Errorf("unsupported cache cleanup mode %q", mode)
	}
}

func isEntryStale(entry cacheIndexEntry, cfg CacheConfig, now time.Time) bool {
	if entry.LastUsedAt.IsZero() {
		return true
	}
	cutoff := now.Add(-time.Duration(cfg.RetentionDays) * 24 * time.Hour)

	return entry.LastUsedAt.Before(cutoff)
}

func (c *Cache) cleanupStaleIfDue(index *cacheIndex) error {
	if !index.LastCleanup.IsZero() &&
		c.clock().Sub(index.LastCleanup) < automaticCleanupInterval {
		return nil
	}
	cfg, _, err := LoadCacheConfig(c.configPath)
	if err != nil {
		return err
	}
	summary := c.planCleanup(*index, cfg, CleanOptions{Mode: CleanupModeStale})
	if err := c.removeEntries(index, summary); err != nil {
		return err
	}
	index.LastCleanup = c.clock().UTC()

	return nil
}

func (c *Cache) removeEntries(index *cacheIndex, summary CleanSummary) error {
	for _, entry := range summary.Entries {
		if err := os.RemoveAll(c.entryDir(entry.ID)); err != nil {
			return err
		}
		delete(index.Entries, entry.ID)
	}

	return nil
}

func (p partialDownloadPlan) summary(dryRun bool) CleanSummary {
	return CleanSummary{
		Mode:           CleanupModePartialDownloads,
		DryRun:         dryRun,
		RemovedEntries: len(p.candidates),
		RemovedBytes:   p.removedBytes,
	}
}

func removePartialDownloads(plan partialDownloadPlan) error {
	for _, candidate := range plan.candidates {
		if err := os.RemoveAll(candidate.path); err != nil {
			return err
		}
	}

	return nil
}

func (c *Cache) planPartialDownloadCleanup() (partialDownloadPlan, error) {
	plan := partialDownloadPlan{}
	root := c.downloadsRoot()
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return plan, nil
		}

		return plan, err
	}
	slices.SortFunc(entries, func(left, right os.DirEntry) int {
		return strings.Compare(left.Name(), right.Name())
	})

	plan.candidates = make([]partialDownloadCandidate, 0, len(entries))
	for _, entry := range entries {
		path := filepath.Join(root, entry.Name())
		size, err := directorySize(path)
		if err != nil {
			return partialDownloadPlan{}, err
		}
		plan.removedBytes += size
		plan.candidates = append(plan.candidates, partialDownloadCandidate{path: path})
	}

	return plan, nil
}

// wipeCacheContents removes cache contents, including unindexed files, while
// preserving lock state for the running operation and resetting artifact
// metadata.
func (c *Cache) wipeCacheContents(index *cacheIndex) error {
	entries, err := os.ReadDir(c.root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			index.Entries = map[string]cacheIndexEntry{}
			return nil
		}

		return err
	}

	for _, entry := range entries {
		if directorymutex.IsMarkerName(entry.Name()) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(c.root, entry.Name())); err != nil {
			return err
		}
	}
	index.Entries = map[string]cacheIndexEntry{}

	return nil
}

func sortedEntryIDs(index cacheIndex) []string {
	keys := make([]string, 0, len(index.Entries))
	for key := range index.Entries {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	return keys
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
