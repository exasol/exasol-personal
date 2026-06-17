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
