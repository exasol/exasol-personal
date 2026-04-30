// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

//nolint:paralleltest // mutates package-level stageDiskCopyCommand hook.
func TestStageDiskImage_StreamCopiesFromCache(t *testing.T) {
	// Given
	disableReflinkCopy(t)
	runtime := New(t.TempDir())
	cacheDir := t.TempDir()
	cachedDiskPath := filepath.Join(cacheDir, "exasol-nano-vm.img")
	if err := os.WriteFile(cachedDiskPath, []byte("disk-image-bytes"), 0o600); err != nil {
		t.Fatalf("expected cached disk fixture to be written, got %v", err)
	}
	if err := runtime.EnsureRoot(); err != nil {
		t.Fatalf("expected EnsureRoot to succeed, got %v", err)
	}

	// When
	stagedPath, err := runtime.StageDiskImage(cachedDiskPath)
	// Then
	if err != nil {
		t.Fatalf("expected first staging to succeed, got %v", err)
	}
	if stagedPath != runtime.Layout().DiskImagePath() {
		t.Fatalf(
			"expected staged path %q, got %q",
			runtime.Layout().DiskImagePath(),
			stagedPath,
		)
	}
	if string(mustReadFile(t, stagedPath)) != "disk-image-bytes" {
		t.Fatal("expected staged disk to match cached source")
	}
}

//nolint:paralleltest // mutates package-level stageDiskCopyCommand hook.
func TestStageDiskImage_ReusesStagedDiskWithSameIdentity(t *testing.T) {
	// Given
	disableReflinkCopy(t)
	runtime := New(t.TempDir())
	cacheDir := t.TempDir()
	cachedDiskPath := filepath.Join(cacheDir, "exasol-nano-vm.img")
	if err := os.WriteFile(cachedDiskPath, []byte("disk-image-bytes"), 0o600); err != nil {
		t.Fatalf("expected cached disk fixture to be written, got %v", err)
	}
	if err := runtime.EnsureRoot(); err != nil {
		t.Fatalf("expected EnsureRoot to succeed, got %v", err)
	}

	stagedPath, err := runtime.StageDiskImage(cachedDiskPath)
	if err != nil {
		t.Fatalf("expected first staging to succeed, got %v", err)
	}
	firstInfo, err := os.Stat(stagedPath)
	if err != nil {
		t.Fatalf("expected staged disk to exist, got %v", err)
	}
	time.Sleep(10 * time.Millisecond)

	// When
	secondPath, err := runtime.StageDiskImage(cachedDiskPath)
	if err != nil {
		t.Fatalf("expected second staging to succeed, got %v", err)
	}
	secondInfo, err := os.Stat(secondPath)
	if err != nil {
		t.Fatalf("expected staged disk to still exist, got %v", err)
	}

	// Then
	if !secondInfo.ModTime().Equal(firstInfo.ModTime()) {
		t.Fatalf(
			"expected staged disk mtime to be unchanged (%v vs %v)",
			firstInfo.ModTime(),
			secondInfo.ModTime(),
		)
	}
}

//nolint:paralleltest // mutates package-level stageDiskCopyCommand hook.
func TestStageDiskImage_RestagesWhenSourceIdentityDiffers(t *testing.T) {
	// Given
	disableReflinkCopy(t)
	runtime := New(t.TempDir())
	cacheDir := t.TempDir()
	firstSource := filepath.Join(cacheDir, "v1.img")
	secondSource := filepath.Join(cacheDir, "v2.img")
	if err := os.WriteFile(firstSource, []byte("v1"), 0o600); err != nil {
		t.Fatalf("expected first source fixture to be written, got %v", err)
	}
	if err := os.WriteFile(secondSource, []byte("v2-different"), 0o600); err != nil {
		t.Fatalf("expected second source fixture to be written, got %v", err)
	}
	if err := runtime.EnsureRoot(); err != nil {
		t.Fatalf("expected EnsureRoot to succeed, got %v", err)
	}

	if _, err := runtime.StageDiskImage(firstSource); err != nil {
		t.Fatalf("expected first staging to succeed, got %v", err)
	}

	// When
	stagedPath, err := runtime.StageDiskImage(secondSource)
	if err != nil {
		t.Fatalf("expected restage to succeed, got %v", err)
	}

	// Then
	contents := string(mustReadFile(t, stagedPath))
	if contents != "v2-different" {
		t.Fatalf("expected restaged disk to match new source, got %q", contents)
	}
}

func disableReflinkCopy(t *testing.T) {
	t.Helper()
	original := stageDiskCopyCommand
	stageDiskCopyCommand = func(string, string) *exec.Cmd {
		return exec.CommandContext(context.Background(), "false")
	}
	t.Cleanup(func() {
		stageDiskCopyCommand = original
	})
}
