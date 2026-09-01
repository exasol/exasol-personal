// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package resource

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestHttpSource_Handles_HTTPURLs(t *testing.T) {
	t.Parallel()

	src := &HttpSource{}
	trueURLs := []string{
		"http://example.com/archive.tar.gz",
		"https://example.com/archive.tar.gz",
		"http://example.com/preset.zip",
		"https://releases.example.com/v1.0/tool",
	}
	for _, url := range trueURLs {
		if !src.Handles(Locator{URL: url}) {
			t.Errorf("Handles(%q) = false, want true", url)
		}
	}
}

func TestHttpSource_Handles_GitURLsExcluded(t *testing.T) {
	t.Parallel()

	src := &HttpSource{}
	falseURLs := []string{
		"https://github.com/org/repo.git",
		"http://github.com/org/repo.git",
		"git@github.com:org/repo.git",
		"git://github.com/org/repo.git",
		"file:///tmp/archive.tar.gz",
		"/local/path/archive.tar.gz",
		"",
	}
	for _, url := range falseURLs {
		if src.Handles(Locator{URL: url}) {
			t.Errorf("Handles(%q) = true, want false", url)
		}
	}
}

func TestHttpSource_Probe_StrongETagIdentifiesContent(t *testing.T) {
	t.Parallel()

	handler := func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("ETag", `"abc123"`)
	}
	server := httptest.NewServer(http.HandlerFunc(handler))
	defer server.Close()

	probe, err := (&HttpSource{}).Probe(context.Background(), Locator{URL: server.URL + "/a.tgz"})
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if !strings.HasSuffix(probe.Identity, `:"abc123"`) {
		t.Fatalf("identity = %q, want the strong tag", probe.Identity)
	}
}

func TestHttpSource_Probe_ScopesETagToLocation(t *testing.T) {
	t.Parallel()

	// Given
	handler := func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("ETag", `"shared"`)
	}
	server := httptest.NewServer(http.HandlerFunc(handler))
	defer server.Close()
	source := &HttpSource{}

	// When
	first, err := source.Probe(context.Background(), Locator{URL: server.URL + "/first.zip"})
	if err != nil {
		t.Fatalf("probe first location: %v", err)
	}
	second, err := source.Probe(context.Background(), Locator{URL: server.URL + "/second.zip"})
	if err != nil {
		t.Fatalf("probe second location: %v", err)
	}

	// Then
	if first.Identity == second.Identity {
		t.Fatalf("expected location-scoped identities, both were %q", first.Identity)
	}
}

func TestHttpSource_Probe_WeakETagIsIgnored(t *testing.T) {
	t.Parallel()

	handler := func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("ETag", `W/"abc123"`)
	}
	server := httptest.NewServer(http.HandlerFunc(handler))
	defer server.Close()

	probe, err := (&HttpSource{}).Probe(context.Background(), Locator{URL: server.URL + "/a.tgz"})
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if probe.Identity != "" {
		t.Fatalf("identity = %q, want empty", probe.Identity)
	}
}

func TestHttpSource_Probe_NoETagStatesNoIdentity(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()

	probe, err := (&HttpSource{}).Probe(context.Background(), Locator{URL: server.URL + "/a.tgz"})
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if probe.Identity != "" {
		t.Fatalf("identity = %q, want empty", probe.Identity)
	}
}

func TestResolver_StrongETagAvoidsRedownload(t *testing.T) {
	t.Parallel()

	var downloads int
	handler := func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("ETag", `"v1"`)
		if request.Method == http.MethodHead {
			return
		}
		downloads++
		_, _ = writer.Write([]byte("payload"))
	}
	server := httptest.NewServer(http.HandlerFunc(handler))
	defer server.Close()

	resolver := newTestResolverForPlatform(t, ResourceSpec{}, t.TempDir(), "linux", "amd64")
	def := ResourceDefinition{
		Artifact: map[string]ArtifactSpec{
			anyPlatformKey: {URL: server.URL + "/payload.bin"},
		},
	}

	if _, err := resolveTestDefinition(context.Background(), resolver, def, "payload"); err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	if _, err := resolveTestDefinition(context.Background(), resolver, def, "payload"); err != nil {
		t.Fatalf("second resolve: %v", err)
	}
	if downloads != 1 {
		t.Fatalf("expected one download, got %d", downloads)
	}
}

//nolint:paralleltest // The process-wide logger must capture both resolutions.
func TestResolver_UnidentifiableArchiveRefetches(t *testing.T) {
	// Given
	var logs bytes.Buffer
	originalLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))
	defer slog.SetDefault(originalLogger)
	var downloads int
	handler := func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodHead {
			return
		}
		downloads++
		_, _ = fmt.Fprintf(writer, "payload-%d", downloads)
	}
	server := httptest.NewServer(http.HandlerFunc(handler))
	defer server.Close()

	resolver := newTestResolverForPlatform(t, ResourceSpec{}, t.TempDir(), "linux", "amd64")
	def := ResourceDefinition{
		Artifact: map[string]ArtifactSpec{
			anyPlatformKey: {URL: server.URL + "/payload.bin"},
		},
	}

	// When
	if _, err := resolveTestDefinition(context.Background(), resolver, def, "payload"); err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	path, err := resolveTestDefinition(context.Background(), resolver, def, "payload")
	if err != nil {
		t.Fatalf("second resolve: %v", err)
	}
	// Then
	if downloads != 2 {
		t.Fatalf("expected a re-fetch, got %d downloads", downloads)
	}
	if !strings.Contains(logs.String(), "re-fetching resource") ||
		!strings.Contains(logs.String(), "result may not be stable") {
		t.Fatalf("expected re-fetch reason in log, got %q", logs.String())
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read refreshed resource: %v", err)
	}
	if string(content) != "payload-2" {
		t.Fatalf("refreshed resource contains %q", content)
	}
}

func TestResolver_ChecksummedDownloadNeedsNoValidatorRequest(t *testing.T) {
	t.Parallel()

	var heads int
	handler := func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodHead {
			heads++
		}
		_, _ = writer.Write([]byte("payload"))
	}
	server := httptest.NewServer(http.HandlerFunc(handler))
	defer server.Close()

	resolver := newTestResolverForPlatform(t, ResourceSpec{}, t.TempDir(), "linux", "amd64")
	def := ResourceDefinition{
		Artifact: map[string]ArtifactSpec{
			anyPlatformKey: {
				URL:    server.URL + "/payload.bin",
				Sha256: sha256OfBytes([]byte("payload")),
			},
		},
	}

	if _, err := resolveTestDefinition(context.Background(), resolver, def, "payload"); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if heads != 0 {
		t.Fatalf("expected no validator request, got %d", heads)
	}
}
