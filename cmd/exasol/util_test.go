// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"testing"

	"github.com/exasol/exasol-personal/internal/deploy"
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

func TestLooksLikeExternalPresetURI(t *testing.T) {
	t.Parallel()

	cases := []struct {
		arg  string
		want bool
	}{
		{"file:///path", true},
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
