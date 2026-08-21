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

func TestParseSpec_LocalGlobTemplateValidates(t *testing.T) {
	t.Parallel()

	raw := []byte(`
infra-presets:
  glob: true
  embed: true
  artifact:
    any:
      url: assets/infrastructure
      resource_path: "*"
`)

	if _, err := ParseSpec(raw); err != nil {
		t.Fatalf("expected a glob template with a local artifact pattern to validate, got %v", err)
	}
}

func TestParseSpec_GitGlobTemplateValidates(t *testing.T) {
	t.Parallel()

	raw := []byte(`
shared-modules:
  glob: true
  embed: true
  artifact:
    any:
      url: https://github.com/org/shared-modules.git
      resource_path: "modules/*"
`)

	if _, err := ParseSpec(raw); err != nil {
		t.Fatalf("expected a glob template with a git artifact pattern to validate, got %v", err)
	}
}

func TestParseSpec_GitGlobTemplateRequiresAPattern(t *testing.T) {
	t.Parallel()

	raw := []byte(`
shared-modules:
  glob: true
  embed: true
  artifact:
    any:
      url: https://github.com/org/shared-modules.git
`)

	_, err := ParseSpec(raw)
	if err == nil || !strings.Contains(err.Error(), "resource_path with a glob pattern") {
		t.Fatalf("expected a missing pattern error, got %v", err)
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
      resource_path: "modules/*"
      sha256: deadbeef
`)

	_, err := ParseSpec(raw)
	if err == nil || !strings.Contains(err.Error(), "must not define sha256") {
		t.Fatalf("expected a git-source checksum rejection, got %v", err)
	}
}

func TestParseSpec_GitGlobTemplateRejectsExtraction(t *testing.T) {
	t.Parallel()

	raw := []byte(`
shared-modules:
  glob: true
  embed: true
  extract: true
  artifact:
    any:
      url: https://github.com/org/shared-modules.git
      resource_path: "modules/*"
`)

	_, err := ParseSpec(raw)
	if err == nil || !strings.Contains(err.Error(), "must not declare extract: true") {
		t.Fatalf("expected an extract-rejection error, got %v", err)
	}
}

func TestParseSpec_ArchiveGlobTemplateValidates(t *testing.T) {
	t.Parallel()

	raw := []byte(`
shared-archives:
  glob: true
  embed: true
  extract: true
  artifact:
    any:
      url: https://example.com/presets.tar.gz
      resource_path: "infra/*"
      sha256: deadbeef
`)

	if _, err := ParseSpec(raw); err != nil {
		t.Fatalf(
			"expected a glob template with an archive artifact pattern to validate, got %v",
			err,
		)
	}
}

func TestParseSpec_ArchiveGlobTemplateRequiresAPattern(t *testing.T) {
	t.Parallel()

	raw := []byte(`
shared-archives:
  glob: true
  embed: true
  extract: true
  artifact:
    any:
      url: https://example.com/presets.tar.gz
      sha256: deadbeef
`)

	_, err := ParseSpec(raw)
	if err == nil || !strings.Contains(err.Error(), "resource_path with a glob pattern") {
		t.Fatalf("expected a missing pattern error, got %v", err)
	}
}

func TestParseSpec_ArchiveGlobTemplateRequiresExtraction(t *testing.T) {
	t.Parallel()

	raw := []byte(`
shared-archives:
  glob: true
  embed: true
  extract: false
  artifact:
    any:
      url: https://example.com/presets.tar.gz
      resource_path: "infra/*"
      sha256: deadbeef
`)

	_, err := ParseSpec(raw)
	if err == nil || !strings.Contains(err.Error(), "extract: true") {
		t.Fatalf("expected an extract:true requirement error, got %v", err)
	}
}
