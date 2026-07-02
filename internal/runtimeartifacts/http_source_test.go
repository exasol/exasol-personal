// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package runtimeartifacts

import (
	"testing"
)

func TestHttpSource_CanFetch_HTTPURLs(t *testing.T) {
	t.Parallel()

	src := &HttpSource{}
	trueURLs := []string{
		"http://example.com/archive.tar.gz",
		"https://example.com/archive.tar.gz",
		"http://example.com/preset.zip",
		"https://releases.example.com/v1.0/tool",
	}
	for _, url := range trueURLs {
		if !src.CanFetch(url) {
			t.Errorf("CanFetch(%q) = false, want true", url)
		}
	}
}

func TestHttpSource_CanFetch_GitURLsExcluded(t *testing.T) {
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
		if src.CanFetch(url) {
			t.Errorf("CanFetch(%q) = true, want false", url)
		}
	}
}
