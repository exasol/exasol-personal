// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package runtimeartifacts

import (
	"strings"
	"testing"
)

func TestParseSpec_FileSourceMayOmitChecksum(t *testing.T) {
	t.Parallel()

	raw := []byte(`
myresource:
  extract: false
  artifact:
    any:
      url: file:///tmp/does-not-need-to-exist-for-parsing
`)

	if _, err := ParseSpec(raw); err != nil {
		t.Fatalf("expected file:// artifact without a checksum to parse, got %v", err)
	}
}

func TestParseSpec_BareLocalPathMayOmitChecksum(t *testing.T) {
	t.Parallel()

	raw := []byte(`
myresource:
  extract: false
  artifact:
    any:
      url: ../relative/local/path
`)

	if _, err := ParseSpec(raw); err != nil {
		t.Fatalf("expected bare local path artifact without a checksum to parse, got %v", err)
	}
}

func TestParseSpec_HTTPSourceStillRequiresChecksum(t *testing.T) {
	t.Parallel()

	raw := []byte(`
myresource:
  extract: false
  artifact:
    any:
      url: https://example.com/tool.tar.gz
`)

	_, err := ParseSpec(raw)
	if err == nil || !strings.Contains(err.Error(), "must define sha256") {
		t.Fatalf("expected missing sha256 error for an http source, got %v", err)
	}
}
