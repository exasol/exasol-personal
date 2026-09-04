// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/directorymutex"
)

func TestRustSLCArchSuffix(t *testing.T) {
	t.Parallel()

	// Given
	for _, testCase := range []struct {
		goarch  string
		want    string
		wantErr bool
	}{
		{"amd64", "", false},
		{"arm64", "-aarch64", false},
		{"386", "", true},
		{"riscv64", "", true},
	} {
		// When
		got, err := rustSLCArchSuffix(testCase.goarch)
		// Then
		if testCase.wantErr {
			if err == nil {
				t.Fatalf("rustSLCArchSuffix(%q): expected an error", testCase.goarch)
			}

			continue
		}
		if err != nil {
			t.Fatalf("rustSLCArchSuffix(%q): unexpected error %v", testCase.goarch, err)
		}
		if got != testCase.want {
			t.Fatalf("rustSLCArchSuffix(%q) = %q, want %q", testCase.goarch, got, testCase.want)
		}
	}
}

func TestLatestRustSLCAssetURLBuildsTheVersionedAssetName(t *testing.T) {
	t.Parallel()

	// Given
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(rustSLCRelease{TagName: "v0.23.0"})
	}))
	defer server.Close()

	for _, testCase := range []struct {
		goarch string
		want   string
	}{
		{"amd64", "https://github.com/exasol-labs/language-container-rs/releases/download/" +
			"v0.23.0/lc-rust-0.23.0.tar.gz"},
		{"arm64", "https://github.com/exasol-labs/language-container-rs/releases/download/" +
			"v0.23.0/lc-rust-0.23.0-aarch64.tar.gz"},
	} {
		// When
		got, err := latestRustSLCAssetURL(context.Background(), server.URL, testCase.goarch)
		// Then
		if err != nil {
			t.Fatalf("%s: unexpected error %v", testCase.goarch, err)
		}
		if got != testCase.want {
			t.Fatalf("%s: url = %q, want %q", testCase.goarch, got, testCase.want)
		}
	}
}

func TestLatestRustSLCAssetURLFailsOnNonOKStatus(t *testing.T) {
	t.Parallel()

	// Given
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// When
	_, err := latestRustSLCAssetURL(context.Background(), server.URL, "amd64")
	// Then
	if err == nil {
		t.Fatal("expected an error for a non-OK response")
	}
	if !strings.Contains(err.Error(), "exasol-labs/language-container-rs") {
		t.Fatalf("error should name the repo, got %v", err)
	}
}

func TestLatestRustSLCAssetURLRejectsAnEmptyTag(t *testing.T) {
	t.Parallel()

	// Given
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(rustSLCRelease{TagName: ""})
	}))
	defer server.Close()

	// When
	_, err := latestRustSLCAssetURL(context.Background(), server.URL, "amd64")
	// Then
	if err == nil {
		t.Fatal("expected an error for an empty tag")
	}
}

func TestLatestRustSLCAssetURLFailsForAnUnsupportedArchitectureWithoutCallingTheAPI(t *testing.T) {
	t.Parallel()

	// Given
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("the release API must not be called for an unsupported architecture")
	}))
	defer server.Close()

	// When
	_, err := latestRustSLCAssetURL(context.Background(), server.URL, "386")
	// Then
	if err == nil {
		t.Fatal("expected an error for an unsupported architecture")
	}
}

func TestResolveRustSLCSourceKeepsAnExplicitSource(t *testing.T) {
	t.Parallel()

	// Given
	for _, source := range []string{"./container.tar.gz", "https://example.com/c.tar.gz"} {
		// When
		got, err := resolveRustSLCSource(context.Background(), source)
		// Then
		if err != nil {
			t.Fatalf("%s: unexpected error %v", source, err)
		}
		if got != source {
			t.Fatalf("resolveRustSLCSource(%q) = %q, want it unchanged", source, got)
		}
	}
}

func TestInstallRustSLCRequiresTheDeploymentLock(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	mutex, err := directorymutex.New(deployment.Root())
	if err != nil {
		t.Fatal(err)
	}

	opts := RustSLCInstallOpts{Source: "container.tar.gz"}

	held := mutex.WithExclusive(context.Background(), nil, func(any) error {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		// When
		_, err := InstallRustSLC(ctx, deployment, opts, false, true, nil)
		// Then
		if !errors.Is(err, ErrDeploymentDirectoryLocked) {
			t.Fatalf("expected ErrDeploymentDirectoryLocked, got %v", err)
		}

		return nil
	})
	if held != nil {
		t.Fatalf("holding the lock failed: %v", held)
	}
}

func TestLatestRustSLCTagFailsOnAnInvalidURL(t *testing.T) {
	t.Parallel()

	// When
	_, err := latestRustSLCTag(context.Background(), "://not-a-valid-url")
	// Then
	if err == nil {
		t.Fatal("expected an error for an invalid release URL")
	}
}

func TestLatestRustSLCTagFailsWhenTheRequestCannotComplete(t *testing.T) {
	t.Parallel()

	// Given: a context that is already cancelled, so the round trip fails without any
	// real network access.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// When
	_, err := latestRustSLCTag(ctx, "https://example.invalid/releases/latest")
	// Then
	if err == nil {
		t.Fatal("expected an error when the request cannot complete")
	}
	if !strings.Contains(err.Error(), "could not reach") {
		t.Fatalf("expected a %q error, got %v", "could not reach", err)
	}
}

func TestLatestRustSLCTagFailsOnMalformedJSON(t *testing.T) {
	t.Parallel()

	// Given
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer server.Close()

	// When
	_, err := latestRustSLCTag(context.Background(), server.URL)
	// Then
	if err == nil {
		t.Fatal("expected an error for a malformed response body")
	}
	if !strings.Contains(err.Error(), "could not parse") {
		t.Fatalf("expected a %q error, got %v", "could not parse", err)
	}
}

func TestInstallRustSLCPropagatesSourceResolutionFailure(t *testing.T) {
	t.Parallel()

	// Given: a cancelled context, so resolving the default (empty) source fails without
	// requiring real network access, and without ever reaching the deployment or the
	// custom-SLC pipeline.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	deployment := config.NewDeploymentDir(t.TempDir())

	// When
	result, err := InstallRustSLC(ctx, deployment, RustSLCInstallOpts{}, false, true, nil)
	// Then
	if err == nil {
		t.Fatal("expected the source-resolution failure to propagate")
	}
	if result != nil {
		t.Fatalf("expected no result on failure, got %+v", result)
	}
}
