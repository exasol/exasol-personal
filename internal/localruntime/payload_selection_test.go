// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	localassets "github.com/exasol/exasol-personal/internal/localruntime/assets"
)

func TestRuntimeEnsurePayloadSelected_PersistsRunAndBootAssetPaths(t *testing.T) {
	// Given
	runtime := New(t.TempDir())
	cacheDir := t.TempDir()
	runBytes := []byte("run")
	kernelBytes := []byte("kernel")
	initrdBytes := []byte("initrd")
	architecture := localPayloadArchitecture()

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
		return "https://example.invalid/metadata.json"
	}
	newPayloadManager = func(metadataURL string, cacheDir string) payloadManager {
		manager := localassets.NewManager(metadataURL, cacheDir)
		manager.HTTPClient = &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				switch req.URL.String() {
				case "https://example.invalid/run":
					return newHTTPResponseBytes(http.StatusOK, runBytes), nil
				case "https://example.invalid/kernel":
					return newHTTPResponseBytes(http.StatusOK, kernelBytes), nil
				case "https://example.invalid/initrd":
					return newHTTPResponseBytes(http.StatusOK, initrdBytes), nil
				case "https://example.invalid/metadata.json":
					return newHTTPResponse(
						http.StatusOK,
						`{"payloads":[{"version":"1.2.3","architecture":"`+
							architecture+
							`","url":"https://example.invalid/run","sha256":"`+
							sha256Hex(runBytes)+
							`","boot":{"kernel":{"url":"https://example.invalid/kernel","sha256":"`+
							sha256Hex(kernelBytes)+
							`"},"initrd":{"url":"https://example.invalid/initrd","sha256":"`+
							sha256Hex(initrdBytes)+
							`"}}}]}`,
					), nil
				default:
					return nil, fmt.Errorf("unexpected URL %s", req.URL)
				}
			}),
		}

		return manager
	}

	// When
	payload, err := runtime.EnsurePayloadSelected(context.Background())

	// Then
	if err != nil {
		t.Fatalf("expected payload selection to succeed, got %v", err)
	}
	if payload.Boot == nil {
		t.Fatal("expected boot assets to be persisted")
	}
	if !isCachedFile(payload.CachePath) {
		t.Fatalf("expected cached run payload path, got %q", payload.CachePath)
	}
	if !isCachedFile(payload.Boot.KernelPath) {
		t.Fatalf("expected cached kernel path, got %q", payload.Boot.KernelPath)
	}
	if !isCachedFile(payload.Boot.InitrdPath) {
		t.Fatalf("expected cached initrd path, got %q", payload.Boot.InitrdPath)
	}
}

func TestRuntimeEnsurePayloadSelected_RejectsMissingBootMetadata(t *testing.T) {
	// Given
	runtime := New(t.TempDir())
	architecture := localPayloadArchitecture()

	originalDefaultCacheDir := defaultPayloadCacheDir
	originalNewManager := newPayloadManager
	originalResolvePayloadMetadataURL := resolvePayloadMetadataURL
	t.Cleanup(func() {
		defaultPayloadCacheDir = originalDefaultCacheDir
		newPayloadManager = originalNewManager
		resolvePayloadMetadataURL = originalResolvePayloadMetadataURL
	})

	defaultPayloadCacheDir = func() (string, error) {
		return t.TempDir(), nil
	}
	resolvePayloadMetadataURL = func() string {
		return "https://example.invalid/metadata.json"
	}
	newPayloadManager = func(metadataURL string, cacheDir string) payloadManager {
		manager := localassets.NewManager(metadataURL, cacheDir)
		manager.HTTPClient = &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.String() == "https://example.invalid/metadata.json" {
					return newHTTPResponse(
						http.StatusOK,
						`{"payloads":[{"version":"1.2.3","architecture":"`+
							architecture+
							`","url":"https://example.invalid/run","sha256":"`+
							sha256Hex([]byte("run"))+
							`"}]}`,
					), nil
				}

				return nil, fmt.Errorf("unexpected URL %s", req.URL)
			}),
		}

		return manager
	}

	// When
	_, err := runtime.EnsurePayloadSelected(context.Background())

	// Then
	if err == nil {
		t.Fatal("expected missing boot metadata to fail")
	}
	if err != nil && err.Error() == "" {
		t.Fatal("expected a descriptive error")
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
