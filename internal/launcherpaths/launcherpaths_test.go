// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package launcherpaths

import (
	"path/filepath"
	"testing"
)

func TestDirPath_JoinsRootAndPersonalUnderBaseDir(t *testing.T) {
	t.Parallel()

	got := DirPath("/home/user")
	want := filepath.Join("/home/user", ".exasol", "personal")
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestRootDirPath_ResolvesUnderTheCurrentUsersHomeDirectory(t *testing.T) {
	t.Parallel()

	got, err := RootDirPath()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if filepath.Base(got) != "personal" || filepath.Base(filepath.Dir(got)) != ".exasol" {
		t.Fatalf("expected a path ending in .exasol/personal, got %q", got)
	}
}
