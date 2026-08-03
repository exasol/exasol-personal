// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import "testing"

func TestParsePresetURI(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		uri        string
		wantClean  string
		wantSubdir string
	}{
		{
			name:       "bare URL has no fragment",
			uri:        "https://example.com/repo.git",
			wantClean:  "https://example.com/repo.git",
			wantSubdir: "",
		},
		{
			name:       "URL with @ref only leaves fragment empty",
			uri:        "https://example.com/repo.git@v1",
			wantClean:  "https://example.com/repo.git@v1",
			wantSubdir: "",
		},
		{
			name:       "URL with fragment only strips fragment",
			uri:        "https://example.com/repo.git#infra/aws",
			wantClean:  "https://example.com/repo.git",
			wantSubdir: "infra/aws",
		},
		{
			name:       "URL with @ref and fragment strips only fragment",
			uri:        "https://example.com/repo.git@v1#infra/aws",
			wantClean:  "https://example.com/repo.git@v1",
			wantSubdir: "infra/aws",
		},
		{
			name:       "empty fragment yields empty subpath",
			uri:        "https://example.com/repo.git#",
			wantClean:  "https://example.com/repo.git",
			wantSubdir: "",
		},
		{
			name:       "percent-encoded slashes are decoded",
			uri:        "https://example.com/repo.git#infra%2Faws",
			wantClean:  "https://example.com/repo.git",
			wantSubdir: "infra/aws",
		},
		{
			name:       "SCP-style git URL with fragment",
			uri:        "git@host.example.com:org/repo.git@main#infra/aws",
			wantClean:  "git@host.example.com:org/repo.git@main",
			wantSubdir: "infra/aws",
		},
		{
			name:       "file archive URI with fragment",
			uri:        "file:///tmp/presets.tar.gz#installation/ubuntu",
			wantClean:  "file:///tmp/presets.tar.gz",
			wantSubdir: "installation/ubuntu",
		},
		{
			name:       "only last # is treated as the fragment separator",
			uri:        "https://example.com/repo.git#a#b",
			wantClean:  "https://example.com/repo.git#a",
			wantSubdir: "b",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			gotClean, gotSubdir := parsePresetURI(test.uri)
			if gotClean != test.wantClean {
				t.Errorf("cleanURI = %q, want %q", gotClean, test.wantClean)
			}
			if gotSubdir != test.wantSubdir {
				t.Errorf("subpath = %q, want %q", gotSubdir, test.wantSubdir)
			}
		})
	}
}
