// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package assets

import (
	"archive/tar"
	"compress/gzip"
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

	"github.com/ulikunitz/xz"
)

var (
	ErrPayloadNotFound            = errors.New("local runtime payload not found")
	ErrPayloadVerificationFailed  = errors.New("local runtime payload verification failed")
	ErrPayloadMetadataUnavailable = errors.New("local runtime payload metadata unavailable")
)

const payloadCacheDirMode = 0o700

type Metadata struct {
	Payloads []Payload `json:"payloads"`
}

type Asset struct {
	URL      string `json:"url"`
	SHA256   string `json:"sha256"`
	Filename string `json:"filename,omitempty"`
	// Format optionally indicates that the downloaded bytes are an archive
	// that should be extracted before use. Supported values: "tar.xz",
	// "tar.gz". Empty (or "raw") means the downloaded file is used as-is.
	Format string `json:"format,omitempty"`
	// ExtractPath is the path inside the archive to extract when Format is
	// set. The extracted regular file becomes the cached asset that this
	// manager returns. Required when Format is non-empty.
	ExtractPath string `json:"extractPath,omitempty"`
}

type BootAssets struct {
	Kernel *Asset `json:"kernel,omitempty"`
	Initrd *Asset `json:"initrd,omitempty"`
}

// Container describes an optional containerized DB runtime that is shipped
// alongside the disk-image payload. The container tarball is loaded by the
// guest's podman + load-shared-container service.
type Container struct {
	Image    *Asset `json:"image,omitempty"`
	ShmSize  string `json:"shmSize,omitempty"`
	Ports    []int  `json:"ports,omitempty"`
	Args     []string `json:"args,omitempty"`
}

type Payload struct {
	Version      string      `json:"version"`
	Architecture string      `json:"architecture"`
	URL          string      `json:"url,omitempty"`
	SHA256       string      `json:"sha256,omitempty"`
	Filename     string      `json:"filename,omitempty"`
	Boot         *BootAssets `json:"boot,omitempty"`
	// Disk, when set, selects the EFI-boot disk-image flow. Mutually exclusive
	// with Boot in practice: payloads either describe kernel+initrd direct
	// boot, or a UEFI disk image.
	Disk *Asset `json:"disk,omitempty"`
	// Container, when set, ships a podman container tarball that the guest
	// loads and runs in addition to (or instead of) executing the .run
	// payload directly. Only consumed by the EFI/disk-image flow.
	Container *Container `json:"container,omitempty"`
}

type CachedBootAssets struct {
	KernelPath string
	InitrdPath string
}

// CachedDiskAsset is the cache result for a UEFI disk-image payload.
type CachedDiskAsset struct {
	Path string
}

// CachedContainerAsset is the cache result for a guest container image.
type CachedContainerAsset struct {
	Path string
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

// EnsureDiskCached downloads and verifies the UEFI disk-image asset, returning
// the local cache path. The disk image is what vz boots from in BootModeEFI.
func (m *Manager) EnsureDiskCached(
	ctx context.Context,
	payload *Payload,
) (*CachedDiskAsset, error) {
	if payload == nil {
		return nil, fmt.Errorf("%w: nil payload", ErrPayloadNotFound)
	}
	if payload.Disk == nil {
		return nil, fmt.Errorf(
			"%w: missing disk image asset for %s/%s",
			ErrPayloadNotFound,
			payload.Version,
			payload.Architecture,
		)
	}

	diskPath, err := m.ensureAssetCached(
		ctx,
		strings.TrimSpace(payload.Version),
		strings.TrimSpace(payload.Architecture),
		"disk",
		payload.Disk,
	)
	if err != nil {
		return nil, err
	}

	return &CachedDiskAsset{Path: diskPath}, nil
}

// EnsureContainerCached downloads and verifies the optional guest container
// image asset, returning the local cache path. Returns nil with no error when
// the payload does not declare a container.
func (m *Manager) EnsureContainerCached(
	ctx context.Context,
	payload *Payload,
) (*CachedContainerAsset, error) {
	if payload == nil {
		return nil, fmt.Errorf("%w: nil payload", ErrPayloadNotFound)
	}
	if payload.Container == nil || payload.Container.Image == nil {
		return nil, nil
	}

	containerPath, err := m.ensureAssetCached(
		ctx,
		strings.TrimSpace(payload.Version),
		strings.TrimSpace(payload.Architecture),
		"container",
		payload.Container.Image,
	)
	if err != nil {
		return nil, err
	}

	return &CachedContainerAsset{Path: containerPath}, nil
}

func (m *Manager) EnsureBootCached(
	ctx context.Context,
	payload *Payload,
) (*CachedBootAssets, error) {
	if payload == nil {
		return nil, fmt.Errorf("%w: nil payload", ErrPayloadNotFound)
	}
	if payload.Boot == nil || payload.Boot.Kernel == nil || payload.Boot.Initrd == nil {
		return nil, fmt.Errorf(
			"%w: missing boot assets for %s/%s",
			ErrPayloadNotFound,
			payload.Version,
			payload.Architecture,
		)
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

	archivePath := filepath.Join(
		m.CacheDir,
		version,
		architecture,
		cachedAssetRelativePath(role, asset.resolvedFilename()),
	)

	extractedPath, err := resolveExtractedPath(archivePath, asset)
	if err != nil {
		return "", err
	}

	if archiveOk, _ := verifyFileSHA256(archivePath, asset.SHA256); archiveOk {
		if extractedPath == "" {
			return archivePath, nil
		}
		if _, statErr := os.Stat(extractedPath); statErr == nil {
			return extractedPath, nil
		}
		if extractErr := extractArchiveEntry(
			archivePath, asset.Format, asset.ExtractPath, extractedPath,
		); extractErr != nil {
			return "", extractErr
		}
		return extractedPath, nil
	}

	if err := os.MkdirAll(filepath.Dir(archivePath), payloadCacheDirMode); err != nil {
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

	tempFile, err := os.CreateTemp(filepath.Dir(archivePath), "payload-*.tmp")
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

	if err := os.Rename(tempPath, archivePath); err != nil {
		return "", fmt.Errorf("failed to store verified payload in cache: %w", err)
	}

	if extractedPath == "" {
		return archivePath, nil
	}

	if err := extractArchiveEntry(
		archivePath, asset.Format, asset.ExtractPath, extractedPath,
	); err != nil {
		return "", err
	}
	return extractedPath, nil
}

// resolveExtractedPath computes where the extracted asset file lives, if the
// asset declares an archive Format. Returns "" for raw assets.
func resolveExtractedPath(archivePath string, asset *Asset) (string, error) {
	format := strings.TrimSpace(asset.Format)
	if format == "" || strings.EqualFold(format, "raw") {
		return "", nil
	}
	if !supportedArchiveFormat(format) {
		return "", fmt.Errorf("%w: unsupported archive format %q", ErrPayloadBundleInvalid, format)
	}
	if strings.TrimSpace(asset.ExtractPath) == "" {
		return "", fmt.Errorf(
			"%w: archive format %q requires extractPath",
			ErrPayloadBundleInvalid, format,
		)
	}

	dir := filepath.Dir(archivePath)
	base := filepath.Base(strings.TrimSpace(asset.ExtractPath))
	if base == "" || base == "." || base == string(filepath.Separator) {
		return "", fmt.Errorf(
			"%w: invalid extractPath %q",
			ErrPayloadBundleInvalid, asset.ExtractPath,
		)
	}
	// Guard against an extractPath whose basename collides with the archive
	// filename — that would overwrite the archive on the next cache hit.
	if base == filepath.Base(archivePath) {
		base = base + ".extracted"
	}
	return filepath.Join(dir, base), nil
}

func supportedArchiveFormat(format string) bool {
	switch strings.ToLower(format) {
	case "tar.xz", "tar.gz":
		return true
	}
	return false
}

// extractArchiveEntry streams a single regular file out of a (possibly
// compressed) tar archive into targetPath atomically. Memory usage is
// bounded; we never buffer the whole archive or entry.
func extractArchiveEntry(archivePath, format, entryPath, targetPath string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open archive: %w", err)
	}
	defer file.Close()

	tarStream, closeFn, err := openTarStream(file, format)
	if err != nil {
		return err
	}
	if closeFn != nil {
		defer closeFn()
	}

	wantPath := filepath.Clean(entryPath)
	reader := tar.NewReader(tarStream)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return fmt.Errorf(
				"%w: archive %q does not contain entry %q",
				ErrPayloadBundleInvalid, archivePath, entryPath,
			)
		}
		if err != nil {
			return fmt.Errorf(
				"%w: failed to read archive %q: %w",
				ErrPayloadBundleInvalid, archivePath, err,
			)
		}
		if filepath.Clean(header.Name) != wantPath {
			continue
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			return fmt.Errorf(
				"%w: entry %q is not a regular file (type %d)",
				ErrPayloadBundleInvalid, entryPath, header.Typeflag,
			)
		}

		if err := writeExtractedFile(reader, header.Size, targetPath); err != nil {
			return err
		}
		return nil
	}
}

func openTarStream(source io.Reader, format string) (io.Reader, func() error, error) {
	switch strings.ToLower(format) {
	case "tar.xz":
		reader, err := xz.NewReader(source)
		if err != nil {
			return nil, nil, fmt.Errorf(
				"%w: not a valid xz stream: %w",
				ErrPayloadBundleInvalid, err,
			)
		}
		return reader, nil, nil
	case "tar.gz":
		reader, err := gzip.NewReader(source)
		if err != nil {
			return nil, nil, fmt.Errorf(
				"%w: not a valid gzip stream: %w",
				ErrPayloadBundleInvalid, err,
			)
		}
		return reader, reader.Close, nil
	default:
		return nil, nil, fmt.Errorf(
			"%w: unsupported archive format %q",
			ErrPayloadBundleInvalid, format,
		)
	}
}

func writeExtractedFile(source io.Reader, size int64, targetPath string) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), payloadCacheDirMode); err != nil {
		return fmt.Errorf("failed to create extraction target dir: %w", err)
	}

	tempPath := targetPath + ".tmp"
	out, err := os.OpenFile(tempPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, bundleFileMode)
	if err != nil {
		return fmt.Errorf("failed to create extraction target: %w", err)
	}

	if _, copyErr := io.CopyN(out, source, size); copyErr != nil {
		_ = out.Close()
		_ = os.Remove(tempPath)
		return fmt.Errorf("failed to extract archive entry: %w", copyErr)
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("failed to finalize extracted asset: %w", err)
	}
	if err := os.Rename(tempPath, targetPath); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("failed to publish extracted asset: %w", err)
	}
	return nil
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

func verifyFileSHA256(filePath string, expected string) (bool, error) {
	file, err := os.Open(filePath)
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
