// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package assets

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
					"url":"https://example.invalid/arm64.run",
					"sha256":"abc",
					"boot":{
						"kernel":{
							"url":"https://example.invalid/kernel",
							"sha256":"def"
						},
						"initrd":{
							"url":"https://example.invalid/initrd",
							"sha256":"ghi"
						}
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
	if payload.Boot == nil || payload.Boot.Kernel == nil || payload.Boot.Initrd == nil {
		t.Fatalf("expected boot assets to be present, got %#v", payload.Boot)
	}
}

func TestManagerEnsureCached_DownloadsVerifiesAndReusesPayload(t *testing.T) {
	t.Parallel()

	// Given
	payloadBytes := []byte("payload-bytes")
	checksum := sha256Hex(payloadBytes)

	manager := NewManager("https://example.invalid/metadata.json", t.TempDir())
	var (
		mutex       sync.Mutex
		downloadHit int
	)
	manager.HTTPClient = testHTTPClient(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != "https://example.invalid/payload.run" {
			return nil, fmt.Errorf("unexpected URL %s", req.URL)
		}

		mutex.Lock()
		downloadHit++
		mutex.Unlock()

		return newHTTPResponseBytes(http.StatusOK, payloadBytes), nil
	})
	payload := &Payload{
		Version:      "1.2.3",
		Architecture: "arm64",
		URL:          "https://example.invalid/payload.run",
		SHA256:       checksum,
	}

	// When
	firstPath, err := manager.EnsureCached(context.Background(), payload)
	if err != nil {
		t.Fatalf("expected first cache fill to succeed, got %v", err)
	}
	secondPath, err := manager.EnsureCached(context.Background(), payload)
	// Then
	if err != nil {
		t.Fatalf("expected cached reuse to succeed, got %v", err)
	}
	if firstPath != secondPath {
		t.Fatalf("expected cached path reuse, got %q then %q", firstPath, secondPath)
	}

	mutex.Lock()
	defer mutex.Unlock()
	if downloadHit != 1 {
		t.Fatalf("expected exactly one download, got %d", downloadHit)
	}
}

func TestManagerEnsureCached_RejectsInvalidChecksum(t *testing.T) {
	t.Parallel()

	// Given
	manager := NewManager("https://example.invalid/metadata.json", t.TempDir())
	manager.HTTPClient = testHTTPClient(func(*http.Request) (*http.Response, error) {
		return newHTTPResponse(http.StatusOK, "payload-bytes"), nil
	})
	payload := &Payload{
		Version:      "1.2.3",
		Architecture: "arm64",
		URL:          "https://example.invalid/payload.run",
		SHA256:       strings.Repeat("0", 64),
	}

	// When
	_, err := manager.EnsureCached(context.Background(), payload)

	// Then
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), ErrPayloadVerificationFailed.Error()) {
		t.Fatalf("expected verification error, got %v", err)
	}
}

func TestManagerEnsureBootCached_DownloadsAndReusesBootAssets(t *testing.T) {
	t.Parallel()

	// Given
	kernelBytes := []byte("kernel")
	initrdBytes := []byte("initrd")
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
		case "https://example.invalid/vmlinux.container":
			return newHTTPResponseBytes(http.StatusOK, kernelBytes), nil
		case "https://example.invalid/ubuntu-initrd.cpio.gz":
			return newHTTPResponseBytes(http.StatusOK, initrdBytes), nil
		default:
			return nil, fmt.Errorf("unexpected URL %s", req.URL)
		}
	})
	payload := &Payload{
		Version:      "1.2.3",
		Architecture: "arm64",
		Boot: &BootAssets{
			Kernel: &Asset{
				URL:    "https://example.invalid/vmlinux.container",
				SHA256: sha256Hex(kernelBytes),
			},
			Initrd: &Asset{
				URL:    "https://example.invalid/ubuntu-initrd.cpio.gz",
				SHA256: sha256Hex(initrdBytes),
			},
		},
	}

	// When
	first, err := manager.EnsureBootCached(context.Background(), payload)
	if err != nil {
		t.Fatalf("expected boot asset cache fill to succeed, got %v", err)
	}
	second, err := manager.EnsureBootCached(context.Background(), payload)
	// Then
	if err != nil {
		t.Fatalf("expected cached boot asset reuse to succeed, got %v", err)
	}
	if first.KernelPath != second.KernelPath || first.InitrdPath != second.InitrdPath {
		t.Fatalf("expected cached boot asset paths to be reused, got %#v then %#v", first, second)
	}
	mutex.Lock()
	defer mutex.Unlock()
	if hits["https://example.invalid/vmlinux.container"] != 1 {
		t.Fatalf(
			"expected exactly one kernel download, got %d",
			hits["https://example.invalid/vmlinux.container"],
		)
	}
	if hits["https://example.invalid/ubuntu-initrd.cpio.gz"] != 1 {
		t.Fatalf(
			"expected exactly one initrd download, got %d",
			hits["https://example.invalid/ubuntu-initrd.cpio.gz"],
		)
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
