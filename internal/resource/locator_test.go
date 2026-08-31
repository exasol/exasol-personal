// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package resource

import (
	"strings"
	"testing"
)

func TestParseURI(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		uri         string
		wantURL     string
		wantRef     string
		wantSubpath string
	}{
		{
			name:    "bare URL carries neither suffix",
			uri:     "https://example.com/repo.git",
			wantURL: "https://example.com/repo.git",
		},
		{
			name:    "ref suffix only",
			uri:     "https://example.com/repo.git@v1",
			wantURL: "https://example.com/repo.git",
			wantRef: "v1",
		},
		{
			name:        "subpath suffix only",
			uri:         "https://example.com/repo.git#infra/aws",
			wantURL:     "https://example.com/repo.git",
			wantSubpath: "infra/aws",
		},
		{
			name:        "ref and subpath together",
			uri:         "https://example.com/repo.git@v1#infra/aws",
			wantURL:     "https://example.com/repo.git",
			wantRef:     "v1",
			wantSubpath: "infra/aws",
		},
		{
			name:    "empty subpath suffix",
			uri:     "https://example.com/repo.git#",
			wantURL: "https://example.com/repo.git",
		},
		{
			name:        "percent-encoded subpath is decoded",
			uri:         "https://example.com/repo.git#infra%2Faws",
			wantURL:     "https://example.com/repo.git",
			wantSubpath: "infra/aws",
		},
		{
			name:        "only the last hash separates the subpath",
			uri:         "https://example.com/repo.git#a#b",
			wantURL:     "https://example.com/repo.git#a",
			wantSubpath: "b",
		},
		{
			name:    "SCP-style git URL keeps its scheme at",
			uri:     "git@github.com:org/repo.git",
			wantURL: "git@github.com:org/repo.git",
		},
		{
			name:        "SCP-style git URL with both suffixes",
			uri:         "git@host.example.com:org/repo.git@main#infra/aws",
			wantURL:     "git@host.example.com:org/repo.git",
			wantRef:     "main",
			wantSubpath: "infra/aws",
		},
		{
			name:    "commit SHA ref",
			uri:     "https://github.com/org/repo.git@a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
			wantURL: "https://github.com/org/repo.git",
			wantRef: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
		},
		{
			name:    "image digest is part of the reference, not a ref",
			uri:     "docker://docker.io/exasol/nano:1@sha256:" + strings.Repeat("a", 64),
			wantURL: "docker://docker.io/exasol/nano:1@sha256:" + strings.Repeat("a", 64),
		},
		{
			name:        "file archive URI with subpath",
			uri:         "file:///tmp/presets.tar.gz#installation/ubuntu",
			wantURL:     "file:///tmp/presets.tar.gz",
			wantSubpath: "installation/ubuntu",
		},
		{
			name:    "remote archive carries no ref",
			uri:     "https://example.com/preset.tar.gz",
			wantURL: "https://example.com/preset.tar.gz",
		},
		{
			name:    "at sign remains in an HTTP location",
			uri:     "https://example.com/preset@v1.zip",
			wantURL: "https://example.com/preset@v1.zip",
		},
		{
			name:    "at sign remains in a local location",
			uri:     "./preset@v1",
			wantURL: "./preset@v1",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := ParseURI(test.uri)
			if got.Locator.URL != test.wantURL {
				t.Errorf("URL = %q, want %q", got.Locator.URL, test.wantURL)
			}
			if got.Locator.Ref != test.wantRef {
				t.Errorf("Ref = %q, want %q", got.Locator.Ref, test.wantRef)
			}
			if got.Subpath != test.wantSubpath {
				t.Errorf("Subpath = %q, want %q", got.Subpath, test.wantSubpath)
			}
		})
	}
}

// A specification declaring url, ref, and subpath as separate fields must
// describe the same source as the equivalent command-line shorthand.
func TestArtifactSpecLocatorMatchesParsedURI(t *testing.T) {
	t.Parallel()

	const uri = "https://example.com/repo.git@v2.1.0#modules/aws"

	fromURI := ParseURI(uri)
	fromFields := ArtifactSpec{
		URL:     "https://example.com/repo.git",
		Ref:     "v2.1.0",
		Subpath: "modules/aws",
	}

	if got := fromFields.Locator(); got != fromURI.Locator {
		t.Errorf("locator from fields = %+v, want %+v", got, fromURI.Locator)
	}
	if fromFields.Subpath != fromURI.Subpath {
		t.Errorf("subpath from fields = %q, want %q", fromFields.Subpath, fromURI.Subpath)
	}
}
