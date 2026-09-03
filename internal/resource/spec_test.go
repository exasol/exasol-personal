// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package resource

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

func TestParseSpec_GitGlobTemplateStillRejectsAChecksum(t *testing.T) {
	t.Parallel()

	raw := []byte(`
shared-modules:
  glob: true
  embed: true
  artifact:
    any:
      url: https://github.com/org/shared-modules.git
      subpath: "modules/*"
      sha256: deadbeef
`)

	_, err := ParseSpec(raw)
	if err == nil || !strings.Contains(err.Error(), "must not define sha256") {
		t.Fatalf("expected a git-source checksum rejection, got %v", err)
	}
}

func TestParseSpec_DigestPinnedImageMayOmitChecksum(t *testing.T) {
	t.Parallel()

	raw := []byte(`
nano:
  artifact:
    any:
      url: docker://docker.io/exasol/nano:1@sha256:` + strings.Repeat("a", 64) + `
      download_path: nano.tar
`)
	if _, err := ParseSpec(raw); err != nil {
		t.Fatalf("expected a digest-pinned image to be accepted, got %v", err)
	}
}

func TestParseSpec_TagOnlyImageRequiresChecksum(t *testing.T) {
	t.Parallel()

	raw := []byte(`
nano:
  artifact:
    any:
      url: docker://docker.io/exasol/nano:1
      download_path: nano.tar
`)
	_, err := ParseSpec(raw)
	if err == nil || !strings.Contains(err.Error(), "sha256") {
		t.Fatalf("expected a tag-only image to require a checksum, got %v", err)
	}
}

func TestParseSpec_RejectsABlankGlobPattern(t *testing.T) {
	t.Parallel()

	raw := []byte(`
presets:
  glob: "   "
  artifact:
    any:
      url: assets/infrastructure
`)
	_, err := ParseSpec(raw)
	if err == nil || !strings.Contains(err.Error(), "glob") {
		t.Fatalf("expected a blank glob pattern to be rejected, got %v", err)
	}
}
