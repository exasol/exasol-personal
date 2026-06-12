// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRunStage_StagesExplicitRunnerPath(t *testing.T) {
	t.Parallel()

	// Given
	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, "launcher")
	if err := os.WriteFile(sourcePath, []byte("runner"), executablePerm); err != nil {
		t.Fatalf("failed to write source runner: %v", err)
	}
	targetPath := filepath.Join(t.TempDir(), "mac-runner-aarch64")

	// When
	err := prepareRunner(context.Background(), []string{
		"-goos", "darwin",
		"-goarch", "arm64",
		"-runner-path", sourcePath,
		"-target", targetPath,
	})
	// Then
	if err != nil {
		t.Fatalf("expected stage to succeed, got %v", err)
	}
	assertFileContent(t, targetPath, "runner")
	assertExecutable(t, targetPath)
}

func TestRunStage_IsIdempotentForExistingTarget(t *testing.T) {
	t.Parallel()

	// Given
	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, "launcher")
	if err := os.WriteFile(sourcePath, []byte("runner"), executablePerm); err != nil {
		t.Fatalf("failed to write source runner: %v", err)
	}
	targetPath := filepath.Join(t.TempDir(), "mac-runner-aarch64")
	if err := os.WriteFile(targetPath, []byte("runner"), 0o600); err != nil {
		t.Fatalf("failed to write target runner: %v", err)
	}

	// When
	err := prepareRunner(context.Background(), []string{
		"-goos", "darwin",
		"-goarch", "arm64",
		"-runner-path", sourcePath,
		"-target", targetPath,
	})
	// Then
	if err != nil {
		t.Fatalf("expected stage to succeed, got %v", err)
	}
	assertFileContent(t, targetPath, "runner")
	assertExecutable(t, targetPath)
}

func TestRunStage_SkipsNonDarwinArm64Target(t *testing.T) {
	t.Parallel()

	// Given
	targetPath := filepath.Join(t.TempDir(), "mac-runner-aarch64")

	// When
	err := prepareRunner(context.Background(), []string{
		"-goos", "linux",
		"-goarch", "amd64",
		"-target", targetPath,
	})
	// Then
	if err != nil {
		t.Fatalf("expected stage to skip unsupported target, got %v", err)
	}
	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		t.Fatalf("expected target to be absent after skip, got %v", err)
	}
}

func TestRunPlaceholder_DoesNotOverwriteExistingTarget(t *testing.T) {
	t.Parallel()

	// Given
	targetPath := filepath.Join(t.TempDir(), "mac-runner-aarch64")
	if err := os.WriteFile(targetPath, []byte("existing"), executablePerm); err != nil {
		t.Fatalf("failed to write existing placeholder: %v", err)
	}

	// When
	err := preparePlaceholder([]string{"-target", targetPath})
	// Then
	if err != nil {
		t.Fatalf("expected placeholder to succeed, got %v", err)
	}
	assertFileContent(t, targetPath, "existing")
}

func assertFileContent(t *testing.T, path, expected string) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}
	if string(data) != expected {
		t.Fatalf("expected %s to contain %q, got %q", path, expected, string(data))
	}
}

func assertExecutable(t *testing.T, path string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("failed to stat %s: %v", path, err)
	}
	if info.Mode().Perm()&0o100 == 0 {
		t.Fatalf("expected %s to be executable, got mode %v", path, info.Mode().Perm())
	}
}

func TestParseRunnerFlags_UsesTargetEnvironment(t *testing.T) {
	t.Setenv("GOOS", "darwin")
	t.Setenv("GOARCH", "arm64")

	// When
	config, err := parseRunnerFlags("stage", nil)
	// Then
	if err != nil {
		t.Fatalf("expected flags to parse, got %v", err)
	}
	if config.targetGOOS != "darwin" || config.targetGOARCH != "arm64" {
		t.Fatalf(
			"expected target from environment, got %s/%s",
			config.targetGOOS,
			config.targetGOARCH,
		)
	}
}
