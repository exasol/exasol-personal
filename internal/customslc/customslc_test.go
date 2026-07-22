// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package customslc_test

import (
	"testing"

	"github.com/exasol/exasol-personal/internal/customslc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeLanguage(t *testing.T) {
	t.Parallel()

	// Given
	for _, testCase := range []struct {
		in   string
		want customslc.Language
		ok   bool
	}{
		{"python", customslc.LanguagePython, true},
		{"PYTHON", customslc.LanguagePython, true},
		{" Java ", customslc.LanguageJava, true},
		{"r", customslc.LanguageR, true},
		{"go", "", false},
		{"", "", false},
	} {
		// When
		got, err := customslc.NormalizeLanguage(testCase.in)
		// Then
		if testCase.ok {
			require.NoError(t, err)
			assert.Equal(t, testCase.want, got)
		} else {
			require.Error(t, err)
		}
	}
}

func TestBuildActivationURI(t *testing.T) {
	t.Parallel()

	// When
	uri := customslc.BuildActivationURI("bfsdefault", "default", "mypy3", customslc.LanguagePython)

	// Then
	assert.Equal(t,
		"localzmq+protobuf:///bfsdefault/default/mypy3?lang=python"+
			"#buckets/bfsdefault/default/mypy3/exaudf/exaudfclient",
		uri,
	)
}

func TestParseAndSerializeRoundTrip(t *testing.T) {
	t.Parallel()

	// Given
	value := "R=builtin_r JAVA=builtin_java PYTHON3=builtin_python3"

	// When
	entries := customslc.ParseScriptLanguages(value)

	// Then
	require.Len(t, entries, 3)
	assert.Equal(t, value, customslc.SerializeScriptLanguages(entries))
}

func TestParseSkipsMalformedTokens(t *testing.T) {
	t.Parallel()

	// When
	entries := customslc.ParseScriptLanguages("PYTHON3=builtin_python3 garbage OTHER=x")

	// Then
	require.Len(t, entries, 2)
	assert.Equal(t, "PYTHON3", entries[0].Alias)
	assert.Equal(t, "OTHER", entries[1].Alias)
}

func TestSetAliasReplacesInPlacePreservingOthers(t *testing.T) {
	t.Parallel()

	// Given
	entries := customslc.ParseScriptLanguages("PYTHON3=builtin_python3 JAVA=builtin_java")

	// When
	updated := customslc.SetAlias(entries, "python3", "localzmq+protobuf:///x?lang=python#y")

	// Then
	// The built-in JAVA entry is untouched; PYTHON3 now points at the custom URI.
	got, ok := customslc.FindAlias(updated, "PYTHON3")
	require.True(t, ok)
	assert.Equal(t, "localzmq+protobuf:///x?lang=python#y", got.URI)
	java, ok := customslc.FindAlias(updated, "JAVA")
	require.True(t, ok)
	assert.Equal(t, "builtin_java", java.URI)
	assert.Len(t, updated, 2)
}

func TestSetAliasAppendsNewAlias(t *testing.T) {
	t.Parallel()

	// Given
	entries := customslc.ParseScriptLanguages("PYTHON3=builtin_python3")

	// When
	updated := customslc.SetAlias(entries, "MYPY3", "localzmq+protobuf:///x?lang=python#y")

	// Then
	require.Len(t, updated, 2)
	_, ok := customslc.FindAlias(updated, "MYPY3")
	assert.True(t, ok)
}

func TestRemoveAlias(t *testing.T) {
	t.Parallel()

	// Given
	entries := customslc.ParseScriptLanguages("PYTHON3=builtin_python3 MYPY3=localzmq://x")

	// When
	updated := customslc.RemoveAlias(entries, "mypy3")

	// Then
	require.Len(t, updated, 1)
	_, ok := customslc.FindAlias(updated, "MYPY3")
	assert.False(t, ok)
}

func TestBuiltinAliasesDistinguishesFromContainerURIs(t *testing.T) {
	t.Parallel()

	// Given
	entries := customslc.ParseScriptLanguages(
		"PYTHON3=builtin_python3 JAVA=builtin_java MYPY3=localzmq+protobuf:///x?lang=python#y",
	)

	// When
	got := customslc.BuiltinAliases(entries)

	// Then
	assert.Equal(t, []string{"JAVA", "PYTHON3"}, got)
}

func TestDirFromURI(t *testing.T) {
	t.Parallel()

	uri := customslc.BuildActivationURI(
		"bfsdefault",
		"default",
		"mypy3-abc123",
		customslc.LanguagePython,
	)
	if got := customslc.DirFromURI(uri); got != "mypy3-abc123" {
		t.Fatalf("DirFromURI = %q, want mypy3-abc123", got)
	}
	if got := customslc.DirFromURI("builtin_python3"); got != "" {
		t.Fatalf("a built-in URI has no dir, got %q", got)
	}
}

func TestIsBuiltinURI(t *testing.T) {
	t.Parallel()

	assert.True(t, customslc.IsBuiltinURI("builtin_python3"))
	assert.False(t, customslc.IsBuiltinURI("localzmq+protobuf:///x?lang=python#y"))
}

func TestOverrideAndRestoreBuiltin(t *testing.T) {
	t.Parallel()

	// Given
	const custom = "localzmq+protobuf:///a?lang=python#a"
	const customUpdated = "localzmq+protobuf:///b?lang=python#b"

	entries := customslc.ParseScriptLanguages("PYTHON3=builtin_python3 JAVA=builtin_java")
	displaced, found := customslc.FindAlias(entries, "PYTHON3")
	require.True(t, found)
	require.True(t, customslc.IsBuiltinURI(displaced.URI))

	// When
	entries = customslc.SetAlias(entries, "PYTHON3", custom)
	entries = customslc.SetAlias(entries, "PYTHON3", customUpdated)
	entries = customslc.SetAlias(entries, "PYTHON3", displaced.URI)

	// Then
	restored, found := customslc.FindAlias(entries, "PYTHON3")
	require.True(t, found)
	assert.Equal(t, "builtin_python3", restored.URI)

	java, found := customslc.FindAlias(entries, "JAVA")
	require.True(t, found)
	assert.Equal(t, "builtin_java", java.URI)
}
