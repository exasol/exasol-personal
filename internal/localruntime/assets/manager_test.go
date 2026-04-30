// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package assets

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
)

func TestManagerResolve_SelectsArchitectureFromMetadata(t *testing.T) {
	t.Parallel()

	// Given
	manager := NewManager("https://example.invalid/metadata.json", t.TempDir())
	manager.HTTPClient = testHTTPClient(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != manager.MetadataURL {
			return nil, fmt.Errorf("unexpected URL %s", req.URL)
		}

		body := `
		{
			"payloads": [
				{
					"version":"1.2.3",
					"architecture":"arm64",
					"disk":{
						"url":"https://example.invalid/disk.img",
						"sha256":"abc",
						"filename":"disk.img"
					},
					"run":{
						"url":"https://example.invalid/db.run",
						"sha256":"def",
						"filename":"db.run"
					}
				}
			]
		}`

		return newHTTPResponse(http.StatusOK, body), nil
	})

	// When
	payload, err := manager.Resolve(context.Background(), "arm64")
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if payload.Version != "1.2.3" {
		t.Fatalf("unexpected version: %q", payload.Version)
	}
	if payload.Architecture != "arm64" {
		t.Fatalf("unexpected architecture: %q", payload.Architecture)
	}
	if payload.Disk == nil ||
		payload.Disk.URL != "https://example.invalid/disk.img" ||
		payload.Disk.SHA256 != "abc" {
		t.Fatalf("expected disk asset to be present, got %#v", payload.Disk)
	}
	if payload.Run == nil ||
		payload.Run.URL != "https://example.invalid/db.run" ||
		payload.Run.SHA256 != "def" {
		t.Fatalf("expected run asset to be present, got %#v", payload.Run)
	}
}

func TestManagerEnsureCached_DownloadsAndReusesBothAssets(t *testing.T) {
	t.Parallel()

	// Given
	diskBytes := []byte("disk-image-bytes")
	runBytes := []byte("run-binary-bytes")

	manager := NewManager("https://example.invalid/metadata.json", t.TempDir())
	var (
		mutex sync.Mutex
		hits  = map[string]int{}
	)
	manager.HTTPClient = testHTTPClient(func(req *http.Request) (*http.Response, error) {
		mutex.Lock()
		hits[req.URL.String()]++
		mutex.Unlock()

		switch req.URL.String() {
		case "https://example.invalid/disk.img":
			return newHTTPResponseBytes(http.StatusOK, diskBytes), nil
		case "https://example.invalid/db.run":
			return newHTTPResponseBytes(http.StatusOK, runBytes), nil
		default:
			return nil, fmt.Errorf("unexpected URL %s", req.URL)
		}
	})
	payload := &Payload{
		Version:      "1.2.3",
		Architecture: "arm64",
		Disk: &Asset{
			URL:      "https://example.invalid/disk.img",
			SHA256:   sha256Hex(diskBytes),
			Filename: "disk.img",
		},
		Run: &Asset{
			URL:      "https://example.invalid/db.run",
			SHA256:   sha256Hex(runBytes),
			Filename: "db.run",
		},
	}

	// When
	first, err := manager.EnsureCached(context.Background(), payload)
	if err != nil {
		t.Fatalf("expected first cache fill to succeed, got %v", err)
	}
	second, err := manager.EnsureCached(context.Background(), payload)
	// Then
	if err != nil {
		t.Fatalf("expected cached reuse to succeed, got %v", err)
	}
	if first.DiskImagePath != second.DiskImagePath {
		t.Fatalf("expected disk path reuse, got %q then %q",
			first.DiskImagePath, second.DiskImagePath)
	}
	if first.RunPath != second.RunPath {
		t.Fatalf("expected run path reuse, got %q then %q",
			first.RunPath, second.RunPath)
	}
	if !strings.HasSuffix(first.DiskImagePath, "/1.2.3/arm64/disk.img") {
		t.Fatalf("unexpected disk cache path: %q", first.DiskImagePath)
	}
	if !strings.HasSuffix(first.RunPath, "/1.2.3/arm64/db.run") {
		t.Fatalf("unexpected run cache path: %q", first.RunPath)
	}

	mutex.Lock()
	defer mutex.Unlock()
	if hits["https://example.invalid/disk.img"] != 1 {
		t.Fatalf("expected exactly one disk download, got %d",
			hits["https://example.invalid/disk.img"])
	}
	if hits["https://example.invalid/db.run"] != 1 {
		t.Fatalf("expected exactly one run download, got %d",
			hits["https://example.invalid/db.run"])
	}
}

func TestManagerEnsureCached_RejectsInvalidChecksum(t *testing.T) {
	t.Parallel()

	// Given
	manager := NewManager("https://example.invalid/metadata.json", t.TempDir())
	manager.HTTPClient = testHTTPClient(func(*http.Request) (*http.Response, error) {
		return newHTTPResponse(http.StatusOK, "disk-image-bytes"), nil
	})
	payload := &Payload{
		Version:      "1.2.3",
		Architecture: "arm64",
		Disk: &Asset{
			URL:      "https://example.invalid/disk.img",
			SHA256:   strings.Repeat("0", 64),
			Filename: "disk.img",
		},
		Run: &Asset{
			URL:      "https://example.invalid/db.run",
			SHA256:   strings.Repeat("0", 64),
			Filename: "db.run",
		},
	}

	// When
	_, err := manager.EnsureCached(context.Background(), payload)

	// Then
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrPayloadVerificationFailed) {
		t.Fatalf("expected verification error, got %v", err)
	}
}

func TestManagerEnsureCached_RejectsPayloadWithoutDiskAsset(t *testing.T) {
	t.Parallel()

	// Given
	manager := NewManager("https://example.invalid/metadata.json", t.TempDir())
	payload := &Payload{
		Version:      "1.2.3",
		Architecture: "arm64",
		Run: &Asset{
			URL:    "https://example.invalid/db.run",
			SHA256: "abc",
		},
	}

	// When
	_, err := manager.EnsureCached(context.Background(), payload)

	// Then
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrPayloadNotFound) {
		t.Fatalf("expected ErrPayloadNotFound, got %v", err)
	}
}

func TestManagerEnsureCached_RejectsPayloadWithoutRunAsset(t *testing.T) {
	t.Parallel()

	// Given
	manager := NewManager("https://example.invalid/metadata.json", t.TempDir())
	payload := &Payload{
		Version:      "1.2.3",
		Architecture: "arm64",
		Disk: &Asset{
			URL:    "https://example.invalid/disk.img",
			SHA256: "abc",
		},
	}

	// When
	_, err := manager.EnsureCached(context.Background(), payload)

	// Then
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrPayloadNotFound) {
		t.Fatalf("expected ErrPayloadNotFound, got %v", err)
	}
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func testHTTPClient(fn roundTripFunc) *http.Client {
	return &http.Client{Transport: fn}
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
