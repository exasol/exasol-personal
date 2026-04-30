// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	localassets "github.com/exasol/exasol-personal/internal/localruntime/assets"
	localstate "github.com/exasol/exasol-personal/internal/localruntime/state"
)

const testPayloadMetadataURL = "https://example.invalid/metadata.json"

//nolint:paralleltest // mutates package-level payload selection hooks.
func TestRuntimeEnsurePayloadSelected_PersistsBothAssetPaths(t *testing.T) {
	// Given
	runtime := New(t.TempDir())
	cacheDir := t.TempDir()
	diskBytes := []byte("disk-image")
	runBytes := []byte("run-binary")
	architecture := localPayloadArchitecture()

	stubPayloadHooks(t, cacheDir, func(req *http.Request) (*http.Response, error) {
		switch req.URL.String() {
		case "https://example.invalid/disk.img":
			return newHTTPResponseBytes(http.StatusOK, diskBytes), nil
		case "https://example.invalid/db.run":
			return newHTTPResponseBytes(http.StatusOK, runBytes), nil
		case testPayloadMetadataURL:
			return newHTTPResponse(
				http.StatusOK,
				`{"payloads":[{"version":"1.2.3","architecture":"`+
					architecture+
					`","disk":{"url":"https://example.invalid/disk.img","sha256":"`+
					sha256Hex(diskBytes)+
					`","filename":"disk.img"},`+
					`"run":{"url":"https://example.invalid/db.run","sha256":"`+
					sha256Hex(runBytes)+
					`","filename":"db.run"}}]}`,
			), nil
		default:
			return nil, fmt.Errorf("unexpected URL %s", req.URL)
		}
	})

	// When
	payload, err := runtime.EnsurePayloadSelected(context.Background())
	// Then
	if err != nil {
		t.Fatalf("expected payload selection to succeed, got %v", err)
	}
	if !isCachedFile(payload.DiskImagePath) {
		t.Fatalf("expected cached disk image path, got %q", payload.DiskImagePath)
	}
	if !isCachedFile(payload.RunPath) {
		t.Fatalf("expected cached run path, got %q", payload.RunPath)
	}
	if !strings.HasSuffix(payload.DiskImagePath, "/1.2.3/"+architecture+"/disk.img") {
		t.Fatalf(
			"expected disk cache layout <version>/<arch>/<filename>, got %q",
			payload.DiskImagePath,
		)
	}
	if !strings.HasSuffix(payload.RunPath, "/1.2.3/"+architecture+"/db.run") {
		t.Fatalf(
			"expected run cache layout <version>/<arch>/<filename>, got %q",
			payload.RunPath,
		)
	}
	if payload.Checksum != sha256Hex(diskBytes) {
		t.Fatalf("expected disk checksum to be persisted, got %q", payload.Checksum)
	}
}

//nolint:paralleltest // mutates package-level payload selection hooks.
func TestRuntimeEnsurePayloadSelected_RejectsMissingDiskMetadata(t *testing.T) {
	// Given
	runtime := New(t.TempDir())
	architecture := localPayloadArchitecture()

	stubPayloadHooks(t, t.TempDir(), func(req *http.Request) (*http.Response, error) {
		if req.URL.String() == testPayloadMetadataURL {
			return newHTTPResponse(
				http.StatusOK,
				`{"payloads":[{"version":"1.2.3","architecture":"`+architecture+
					`","run":{"url":"https://example.invalid/db.run","sha256":"abc"}}]}`,
			), nil
		}

		return nil, fmt.Errorf("unexpected URL %s", req.URL)
	})

	// When
	_, err := runtime.EnsurePayloadSelected(context.Background())

	// Then
	if err == nil {
		t.Fatal("expected missing disk metadata to fail")
	}
	if !errors.Is(err, ErrPayloadAssetMissing) {
		t.Fatalf("expected ErrPayloadAssetMissing, got %v", err)
	}
}

//nolint:paralleltest // mutates package-level payload selection hooks.
func TestRuntimeEnsurePayloadSelected_RejectsMissingRunMetadata(t *testing.T) {
	// Given
	runtime := New(t.TempDir())
	architecture := localPayloadArchitecture()

	stubPayloadHooks(t, t.TempDir(), func(req *http.Request) (*http.Response, error) {
		if req.URL.String() == testPayloadMetadataURL {
			return newHTTPResponse(
				http.StatusOK,
				`{"payloads":[{"version":"1.2.3","architecture":"`+architecture+
					`","disk":{"url":"https://example.invalid/disk.img","sha256":"abc"}}]}`,
			), nil
		}

		return nil, fmt.Errorf("unexpected URL %s", req.URL)
	})

	// When
	_, err := runtime.EnsurePayloadSelected(context.Background())

	// Then
	if err == nil {
		t.Fatal("expected missing run metadata to fail")
	}
	if !errors.Is(err, ErrPayloadAssetMissing) {
		t.Fatalf("expected ErrPayloadAssetMissing, got %v", err)
	}
}

func TestCachedPayloadRef_NilWhenDiskImageMissing(t *testing.T) {
	t.Parallel()

	// Given
	state := &localstate.State{
		Payload: &localstate.PayloadRef{
			Version:       "1.2.3",
			Architecture:  "arm64",
			Checksum:      "abc",
			DiskImagePath: "/nonexistent/disk.img",
			RunPath:       "/nonexistent/db.run",
		},
	}

	// When
	ref := cachedPayloadRef(state)

	// Then
	if ref != nil {
		t.Fatalf("expected nil cached ref when disk image absent, got %#v", ref)
	}
}

func TestCachedPayloadRef_NilWhenRunPathMissing(t *testing.T) {
	t.Parallel()

	// Given
	tmp := t.TempDir()
	diskPath := filepath.Join(tmp, "disk.img")
	if err := os.WriteFile(diskPath, []byte("disk"), 0o600); err != nil {
		t.Fatalf("expected disk fixture to be written, got %v", err)
	}
	state := &localstate.State{
		Payload: &localstate.PayloadRef{
			DiskImagePath: diskPath,
			RunPath:       filepath.Join(tmp, "missing.run"),
		},
	}

	// When
	ref := cachedPayloadRef(state)

	// Then
	if ref != nil {
		t.Fatalf("expected nil cached ref when run path absent, got %#v", ref)
	}
}

func stubPayloadHooks(
	t *testing.T,
	cacheDir string,
	respond func(*http.Request) (*http.Response, error),
) {
	t.Helper()
	originalDefaultCacheDir := defaultPayloadCacheDir
	originalNewManager := newPayloadManager
	originalResolvePayloadMetadataURL := resolvePayloadMetadataURL
	t.Cleanup(func() {
		defaultPayloadCacheDir = originalDefaultCacheDir
		newPayloadManager = originalNewManager
		resolvePayloadMetadataURL = originalResolvePayloadMetadataURL
	})

	defaultPayloadCacheDir = func() (string, error) {
		return cacheDir, nil
	}
	resolvePayloadMetadataURL = func() string {
		return testPayloadMetadataURL
	}
	newPayloadManager = func(metadataURL string, cacheDir string) payloadManager {
		manager := localassets.NewManager(metadataURL, cacheDir)
		manager.HTTPClient = &http.Client{Transport: roundTripFunc(respond)}

		return manager
	}
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func newHTTPResponse(statusCode int, body string) *http.Response {
	return newHTTPResponseBytes(statusCode, []byte(body))
}

func newHTTPResponseBytes(statusCode int, body []byte) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Status:     fmt.Sprintf("%d %s", statusCode, http.StatusText(statusCode)),
		Body:       io.NopCloser(strings.NewReader(string(body))),
		Header:     make(http.Header),
	}
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
