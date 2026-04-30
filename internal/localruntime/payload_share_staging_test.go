// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStagePayloadShare_WritesStartScriptAndStagesRun(t *testing.T) {
	t.Parallel()

	// Given
	runtime := New(t.TempDir())
	cacheDir := t.TempDir()
	cachedRun := filepath.Join(cacheDir, "exasol-nano-db.run")
	if err := os.WriteFile(cachedRun, []byte("run-bytes"), 0o600); err != nil {
		t.Fatalf("expected run fixture to be written, got %v", err)
	}
	if err := runtime.EnsureRoot(); err != nil {
		t.Fatalf("expected EnsureRoot to succeed, got %v", err)
	}

	// When
	if err := runtime.StagePayloadShare(cachedRun); err != nil {
		t.Fatalf("expected staging to succeed, got %v", err)
	}

	// Then
	stagedRun := runtime.Layout().PayloadRunPath()
	if string(mustReadFile(t, stagedRun)) != "run-bytes" {
		t.Fatal("expected staged run content to match cache")
	}
	stagedRunInfo, err := os.Stat(stagedRun)
	if err != nil {
		t.Fatalf("expected staged run to exist, got %v", err)
	}
	if stagedRunInfo.Mode().Perm()&0o111 == 0 {
		t.Fatalf("expected staged run executable, got mode %v", stagedRunInfo.Mode())
	}

	stagedStart := runtime.Layout().PayloadStartScriptPath()
	stagedStartInfo, err := os.Stat(stagedStart)
	if err != nil {
		t.Fatalf("expected staged start script to exist, got %v", err)
	}
	if stagedStartInfo.Mode().Perm()&0o111 == 0 {
		t.Fatalf("expected start script executable, got mode %v", stagedStartInfo.Mode())
	}
	if len(mustReadFile(t, stagedStart)) == 0 {
		t.Fatal("expected start script content to be non-empty")
	}
}

func TestStagePayloadShare_SkipsRunCopyWhenChecksumMatches(t *testing.T) {
	t.Parallel()

	// Given
	runtime := New(t.TempDir())
	cacheDir := t.TempDir()
	cachedRun := filepath.Join(cacheDir, "exasol-nano-db.run")
	if err := os.WriteFile(cachedRun, []byte("run-bytes"), 0o600); err != nil {
		t.Fatalf("expected run fixture to be written, got %v", err)
	}
	if err := runtime.EnsureRoot(); err != nil {
		t.Fatalf("expected EnsureRoot to succeed, got %v", err)
	}

	if err := runtime.StagePayloadShare(cachedRun); err != nil {
		t.Fatalf("expected first staging to succeed, got %v", err)
	}
	stagedRun := runtime.Layout().PayloadRunPath()
	firstRunInfo, err := os.Stat(stagedRun)
	if err != nil {
		t.Fatalf("expected staged run to exist, got %v", err)
	}
	stagedStart := runtime.Layout().PayloadStartScriptPath()
	firstStartInfo, err := os.Stat(stagedStart)
	if err != nil {
		t.Fatalf("expected start script to exist, got %v", err)
	}
	time.Sleep(20 * time.Millisecond)

	// When
	if err := runtime.StagePayloadShare(cachedRun); err != nil {
		t.Fatalf("expected second staging to succeed, got %v", err)
	}

	// Then
	secondRunInfo, err := os.Stat(stagedRun)
	if err != nil {
		t.Fatalf("expected staged run to still exist, got %v", err)
	}
	if !secondRunInfo.ModTime().Equal(firstRunInfo.ModTime()) {
		t.Fatalf(
			"expected staged run mtime unchanged when checksum matches (%v vs %v)",
			firstRunInfo.ModTime(),
			secondRunInfo.ModTime(),
		)
	}

	secondStartInfo, err := os.Stat(stagedStart)
	if err != nil {
		t.Fatalf("expected start script to still exist, got %v", err)
	}
	if secondStartInfo.ModTime().Equal(firstStartInfo.ModTime()) {
		t.Fatalf(
			"expected start script mtime to update on every stage (was %v, now %v)",
			firstStartInfo.ModTime(),
			secondStartInfo.ModTime(),
		)
	}
}

func TestStagePayloadShare_RestagesWhenChecksumDiffers(t *testing.T) {
	t.Parallel()

	// Given
	runtime := New(t.TempDir())
	cacheDir := t.TempDir()
	cachedRunV1 := filepath.Join(cacheDir, "v1.run")
	cachedRunV2 := filepath.Join(cacheDir, "v2.run")
	if err := os.WriteFile(cachedRunV1, []byte("v1"), 0o600); err != nil {
		t.Fatalf("expected v1 fixture, got %v", err)
	}
	if err := os.WriteFile(cachedRunV2, []byte("v2-different"), 0o600); err != nil {
		t.Fatalf("expected v2 fixture, got %v", err)
	}
	if err := runtime.EnsureRoot(); err != nil {
		t.Fatalf("expected EnsureRoot to succeed, got %v", err)
	}

	if err := runtime.StagePayloadShare(cachedRunV1); err != nil {
		t.Fatalf("expected v1 staging to succeed, got %v", err)
	}

	// When
	if err := runtime.StagePayloadShare(cachedRunV2); err != nil {
		t.Fatalf("expected v2 restage to succeed, got %v", err)
	}

	// Then
	stagedRun := runtime.Layout().PayloadRunPath()
	if string(mustReadFile(t, stagedRun)) != "v2-different" {
		t.Fatalf("expected restaged run to match v2 contents, got %q",
			string(mustReadFile(t, stagedRun)))
	}
}
