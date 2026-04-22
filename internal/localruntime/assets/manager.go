// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package assets

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
)

var (
	ErrPayloadNotFound            = errors.New("local runtime payload not found")
	ErrPayloadVerificationFailed  = errors.New("local runtime payload verification failed")
	ErrPayloadMetadataUnavailable = errors.New("local runtime payload metadata unavailable")
)

type Metadata struct {
	Payloads []Payload `json:"payloads"`
}

type Asset struct {
	URL      string `json:"url"`
	SHA256   string `json:"sha256"`
	Filename string `json:"filename,omitempty"`
}

type BootAssets struct {
	Kernel *Asset `json:"kernel,omitempty"`
	Initrd *Asset `json:"initrd,omitempty"`
}

type Payload struct {
	Version      string      `json:"version"`
	Architecture string      `json:"architecture"`
	URL          string      `json:"url,omitempty"`
	SHA256       string      `json:"sha256,omitempty"`
	Filename     string      `json:"filename,omitempty"`
	Boot         *BootAssets `json:"boot,omitempty"`
}

type CachedBootAssets struct {
	KernelPath string
	InitrdPath string
}

func (m *Metadata) Resolve(architecture string) (*Payload, error) {
	for _, payload := range m.Payloads {
		if payload.Architecture == architecture {
			result := payload
			return &result, nil
		}
	}

	return nil, fmt.Errorf("%w: architecture %q", ErrPayloadNotFound, architecture)
}

type Manager struct {
	MetadataURL string
	CacheDir    string
	HTTPClient  *http.Client
}

func NewManager(metadataURL string, cacheDir string) *Manager {
	return &Manager{
		MetadataURL: metadataURL,
		CacheDir:    cacheDir,
		HTTPClient:  http.DefaultClient,
	}
}

func DefaultCacheDir() (string, error) {
	cacheRoot, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("failed to resolve user cache dir: %w", err)
	}

	return filepath.Join(cacheRoot, "exasol-personal", "localruntime", "payloads"), nil
}

func (m *Manager) Resolve(ctx context.Context, architecture string) (*Payload, error) {
	metadata, err := m.fetchMetadata(ctx)
	if err != nil {
		return nil, err
	}

	return metadata.Resolve(architecture)
}

func (m *Manager) EnsureCached(ctx context.Context, payload *Payload) (string, error) {
	if payload == nil {
		return "", fmt.Errorf("%w: nil payload", ErrPayloadNotFound)
	}

	return m.ensureAssetCached(
		ctx,
		strings.TrimSpace(payload.Version),
		strings.TrimSpace(payload.Architecture),
		"",
		&Asset{
			URL:      payload.URL,
			SHA256:   payload.SHA256,
			Filename: payload.Filename,
		},
	)
}

func (m *Manager) EnsureBootCached(ctx context.Context, payload *Payload) (*CachedBootAssets, error) {
	if payload == nil {
		return nil, fmt.Errorf("%w: nil payload", ErrPayloadNotFound)
	}
	if payload.Boot == nil || payload.Boot.Kernel == nil || payload.Boot.Initrd == nil {
		return nil, fmt.Errorf("%w: missing boot assets for %s/%s", ErrPayloadNotFound, payload.Version, payload.Architecture)
	}

	kernelPath, err := m.ensureAssetCached(
		ctx,
		strings.TrimSpace(payload.Version),
		strings.TrimSpace(payload.Architecture),
		"boot",
		payload.Boot.Kernel,
	)
	if err != nil {
		return nil, err
	}

	initrdPath, err := m.ensureAssetCached(
		ctx,
		strings.TrimSpace(payload.Version),
		strings.TrimSpace(payload.Architecture),
		"boot",
		payload.Boot.Initrd,
	)
	if err != nil {
		return nil, err
	}

	return &CachedBootAssets{
		KernelPath: kernelPath,
		InitrdPath: initrdPath,
	}, nil
}

func (m *Manager) ensureAssetCached(
	ctx context.Context,
	version string,
	architecture string,
	role string,
	asset *Asset,
) (string, error) {
	if asset == nil {
		return "", fmt.Errorf("%w: nil asset", ErrPayloadNotFound)
	}
	if strings.TrimSpace(asset.URL) == "" {
		return "", fmt.Errorf("%w: empty asset URL", ErrPayloadNotFound)
	}
	if strings.TrimSpace(asset.SHA256) == "" {
		return "", fmt.Errorf("%w: empty asset checksum", ErrPayloadVerificationFailed)
	}

	cachePath := filepath.Join(
		m.CacheDir,
		version,
		architecture,
		cachedAssetRelativePath(role, asset.resolvedFilename()),
	)

	if ok, err := verifyFileSHA256(cachePath, asset.SHA256); err == nil && ok {
		return cachePath, nil
	}

	if err := os.MkdirAll(filepath.Dir(cachePath), 0o700); err != nil {
		return "", fmt.Errorf("failed to create payload cache dir: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.URL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to build payload download request: %w", err)
	}
	resp, err := m.client().Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to download payload: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download payload: unexpected HTTP status %s", resp.Status)
	}

	tempFile, err := os.CreateTemp(filepath.Dir(cachePath), "payload-*.tmp")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary payload file: %w", err)
	}
	tempPath := tempFile.Name()

	defer func() {
		_ = tempFile.Close()
		_ = os.Remove(tempPath)
	}()

	if _, err := io.Copy(tempFile, resp.Body); err != nil {
		return "", fmt.Errorf("failed to write payload download: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return "", fmt.Errorf("failed to finalize payload download: %w", err)
	}

	ok, err := verifyFileSHA256(tempPath, asset.SHA256)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf(
			"%w: expected sha256 %s for %s",
			ErrPayloadVerificationFailed,
			asset.SHA256,
			asset.resolvedFilename(),
		)
	}

	if err := os.Rename(tempPath, cachePath); err != nil {
		return "", fmt.Errorf("failed to store verified payload in cache: %w", err)
	}

	return cachePath, nil
}

func (m *Manager) fetchMetadata(ctx context.Context) (*Metadata, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.MetadataURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build payload metadata request: %w", err)
	}

	resp, err := m.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrPayloadMetadataUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(
			"%w: unexpected HTTP status %s",
			ErrPayloadMetadataUnavailable,
			resp.Status,
		)
	}

	var metadata Metadata
	if err := json.NewDecoder(resp.Body).Decode(&metadata); err != nil {
		return nil, fmt.Errorf("failed to decode payload metadata: %w", err)
	}

	return &metadata, nil
}

func (m *Manager) client() *http.Client {
	if m.HTTPClient != nil {
		return m.HTTPClient
	}

	return http.DefaultClient
}

func cachedAssetRelativePath(role string, filename string) string {
	if strings.TrimSpace(role) == "" {
		return filename
	}

	return filepath.Join(role, filename)
}

func (a *Asset) resolvedFilename() string {
	if strings.TrimSpace(a.Filename) != "" {
		return a.Filename
	}

	parsedURL, err := url.Parse(a.URL)
	if err == nil {
		base := path.Base(parsedURL.Path)
		if base != "" && base != "." && base != "/" {
			return base
		}
	}

	return "asset.bin"
}

func verifyFileSHA256(path string, expected string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}

		return false, fmt.Errorf("failed to open payload file for verification: %w", err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return false, fmt.Errorf("failed to hash payload file: %w", err)
	}

	actual := hex.EncodeToString(hash.Sum(nil))

	return strings.EqualFold(actual, expected), nil
}
