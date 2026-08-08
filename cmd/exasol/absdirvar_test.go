// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewAbsDirValue_EmptyDefaultLeavesTargetEmpty(t *testing.T) {
	t.Parallel()

	var target string
	NewAbsDirValue(&target, "")

	if target != "" {
		t.Fatalf("expected an empty target for an empty default, got %q", target)
	}
}

func TestNewAbsDirValue_ResolvesRelativeDefaultToAbsolute(t *testing.T) {
	t.Parallel()

	var target string
	NewAbsDirValue(&target, ".")

	if !filepath.IsAbs(target) {
		t.Fatalf("expected an absolute path, got %q", target)
	}
}

func TestAbsDirValue_StringReturnsTargetOrEmptyWhenNil(t *testing.T) {
	t.Parallel()

	value := &AbsDirValue{}
	if got := value.String(); got != "" {
		t.Fatalf("expected an empty string for a nil target, got %q", got)
	}

	target := "/some/path"
	value = &AbsDirValue{target: &target}
	if got := value.String(); got != "/some/path" {
		t.Fatalf("expected the target value, got %q", got)
	}
}

func TestAbsDirValue_TypeIsFilePath(t *testing.T) {
	t.Parallel()

	if got := (&AbsDirValue{}).Type(); got != "file-path" {
		t.Fatalf("expected 'file-path', got %q", got)
	}
}

func TestAbsDirValue_SetResolvesToAbsolutePath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	var target string
	value := &AbsDirValue{target: &target}

	if err := value.Set(dir); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if target != filepath.Clean(dir) {
		t.Fatalf("expected the cleaned absolute path, got %q", target)
	}
}

func TestAbsDirValue_SetAcceptsNonExistentPath(t *testing.T) {
	t.Parallel()

	var target string
	value := &AbsDirValue{target: &target}
	notYetCreated := filepath.Join(t.TempDir(), "not-yet-created")

	if err := value.Set(notYetCreated); err != nil {
		t.Fatalf("expected a not-yet-existing path to be accepted, got %v", err)
	}
}

func TestAbsDirValue_SetRejectsExistingFile(t *testing.T) {
	t.Parallel()

	filePath := filepath.Join(t.TempDir(), "a-file")
	if err := os.WriteFile(filePath, []byte("x"), 0o600); err != nil {
		t.Fatalf("failed to write file fixture: %v", err)
	}

	var target string
	value := &AbsDirValue{target: &target}

	if err := value.Set(filePath); err == nil {
		t.Fatal("expected an error when the path is an existing regular file")
	}
}
