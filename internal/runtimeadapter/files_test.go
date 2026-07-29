// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package runtimeadapter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemovePersonalDataRejectsSymlinkedDataRoot(t *testing.T) {
	t.Parallel()

	deployment := t.TempDir()
	outside := t.TempDir()
	localPath := filepath.Join(deployment, "local")
	if err := os.MkdirAll(localPath, privateDirMode); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(localPath, "data")); err != nil {
		t.Fatal(err)
	}
	outsideData := filepath.Join(outside, "exa")
	if err := os.MkdirAll(outsideData, privateDirMode); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(outsideData, "preserve")
	if err := os.WriteFile(marker, []byte("preserve"), privateFileMode); err != nil {
		t.Fatal(err)
	}

	err := RemovePersonalData(
		deployment,
		filepath.Join(deployment, "local", "data", "exa"),
	)
	if err == nil || !strings.Contains(err.Error(), "symlinked Personal data root") {
		t.Fatalf("expected symlinked root rejection, got %v", err)
	}
	if data, readErr := os.ReadFile(marker); readErr != nil || string(data) != "preserve" {
		t.Fatalf("outside data was modified: data=%q err=%v", data, readErr)
	}
}

func TestRemovePersonalDataRemovesOnlyCanonicalOwnedTarget(t *testing.T) {
	t.Parallel()

	deployment := t.TempDir()
	dataPath := filepath.Join(deployment, "local", "data", "exa")
	sibling := filepath.Join(deployment, "local", "data", "keep")
	if err := os.MkdirAll(dataPath, privateDirMode); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sibling, privateDirMode); err != nil {
		t.Fatal(err)
	}

	if err := RemovePersonalData(deployment, dataPath); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dataPath); !os.IsNotExist(err) {
		t.Fatalf("expected target data to be removed, got %v", err)
	}
	if _, err := os.Stat(sibling); err != nil {
		t.Fatalf("expected sibling data to survive, got %v", err)
	}
}
