// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/exasol/exasol-personal/internal/presets"
)

func TestLooksLikePathPresetArg(t *testing.T) {
	t.Parallel()

	cases := []struct {
		arg      string
		wantPath bool
	}{
		{"aws", false},
		{"ubuntu", false},
		{"my-preset", false},
		{"./local", true},
		{"/abs/path", true},
		{"~/home", true},
		{`C:\Windows\path`, true},
	}
	for _, tc := range cases {
		got := looksLikePathPresetArg(tc.arg)
		if got != tc.wantPath {
			t.Errorf("looksLikePathPresetArg(%q) = %v, want %v", tc.arg, got, tc.wantPath)
		}
	}
}

//nolint:paralleltest // t.Chdir changes process state.
func TestResolvePresetRefMatchesKnownNamesBeforeLocations(t *testing.T) {
	// Given
	cwd := t.TempDir()
	if err := os.Mkdir(filepath.Join(cwd, "aws"), 0o700); err != nil {
		t.Fatalf("create same-named local directory: %v", err)
	}
	t.Chdir(cwd)
	ctx := testResolverContext(t)

	// When
	ref, err := resolvePresetRef(ctx, "aws", presets.PresetTypeInfrastructure)
	// Then
	if err != nil {
		t.Fatalf("resolve known preset: %v", err)
	}
	if ref.Name != "aws" || ref.Path != "" {
		t.Fatalf("expected embedded aws preset, got %#v", ref)
	}
}

func TestResolvePresetRefListsKnownNamesForUnknownPlainName(t *testing.T) {
	t.Parallel()

	// Given
	ctx := testResolverContext(t)

	// When
	_, err := resolvePresetRef(ctx, "unknown", presets.PresetTypeInfrastructure)

	// Then
	if err == nil || !strings.Contains(err.Error(), "unknown") ||
		!strings.Contains(err.Error(), "aws") {
		t.Fatalf("expected descriptive unknown-preset error, got %v", err)
	}
}

func TestLooksLikeExternalPresetURI(t *testing.T) {
	t.Parallel()

	cases := []struct {
		arg  string
		want bool
	}{
		{"file:///path", true},
		{"https://example.com/repo.git", true},
		{"http://example.com/repo.git", true},
		{"git://example.com/repo.git", true},
		{"git@github.com:org/repo.git", true},
		{"aws", false},
		{"./local", false},
		{"/abs/path", false},
		{"ubuntu", false},
	}
	for _, tc := range cases {
		got := deploy.IsExternalPresetURI(tc.arg)
		if got != tc.want {
			t.Errorf("IsExternalPresetURI(%q) = %v, want %v", tc.arg, got, tc.want)
		}
	}
}
