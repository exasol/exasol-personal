// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

// Package customslc is kept free of deployment/backend dependencies so its SCRIPT_LANGUAGES
// and activation-URI logic stays pure and unit-testable.
package customslc

import (
	"fmt"
	"sort"
	"strings"
)

const aliasURIParts = 2

const clientRelPath = "exaudf/exaudfclient"

type AliasEntry struct {
	Alias string
	URI   string
}

// Language is the "?lang=" URI value (e.g. python), not the alias.
type Language string

const (
	LanguagePython Language = "python"
	LanguageJava   Language = "java"
	LanguageR      Language = "r"
)

func NormalizeLanguage(raw string) (Language, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(LanguagePython):
		return LanguagePython, nil
	case string(LanguageJava):
		return LanguageJava, nil
	case string(LanguageR):
		return LanguageR, nil
	default:
		return "", fmt.Errorf(
			"unsupported language %q; supported languages are: python, java, r",
			raw,
		)
	}
}

// Upper-cased so comparisons and stored values match how the database reports identifiers.
func NormalizeAlias(alias string) string {
	return strings.ToUpper(strings.TrimSpace(alias))
}

// The protocol path and the client-path fragment differ only by a leading slash, matching
// the documented Exasol activation URI shape.
func BuildActivationURI(bucketFS, bucket, dir string, language Language) string {
	path := bucketFS + "/" + bucket + "/" + dir

	return fmt.Sprintf(
		"localzmq+protobuf:///%s?lang=%s#buckets/%s/%s",
		path, language, path, clientRelPath,
	)
}

// Tokens without an '=' are skipped rather than guessed at.
func ParseScriptLanguages(value string) []AliasEntry {
	fields := strings.Fields(value)
	entries := make([]AliasEntry, 0, len(fields))
	for _, field := range fields {
		parts := strings.SplitN(field, "=", aliasURIParts)
		if len(parts) != aliasURIParts {
			continue
		}
		entries = append(entries, AliasEntry{Alias: NormalizeAlias(parts[0]), URI: parts[1]})
	}

	return entries
}

func SerializeScriptLanguages(entries []AliasEntry) string {
	parts := make([]string, 0, len(entries))
	for _, entry := range entries {
		parts = append(parts, entry.Alias+"="+entry.URI)
	}

	return strings.Join(parts, " ")
}

// Every other entry (builtins included) is preserved: read-merge-write, never a full replace.
func SetAlias(entries []AliasEntry, alias, uri string) []AliasEntry {
	normalized := NormalizeAlias(alias)
	updated := make([]AliasEntry, 0, len(entries)+1)
	found := false
	for _, entry := range entries {
		if entry.Alias == normalized {
			updated = append(updated, AliasEntry{Alias: normalized, URI: uri})
			found = true

			continue
		}
		updated = append(updated, entry)
	}
	if !found {
		updated = append(updated, AliasEntry{Alias: normalized, URI: uri})
	}

	return updated
}

func RemoveAlias(entries []AliasEntry, alias string) []AliasEntry {
	normalized := NormalizeAlias(alias)
	updated := make([]AliasEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Alias == normalized {
			continue
		}
		updated = append(updated, entry)
	}

	return updated
}

func FindAlias(entries []AliasEntry, alias string) (AliasEntry, bool) {
	normalized := NormalizeAlias(alias)
	for _, entry := range entries {
		if entry.Alias == normalized {
			return entry, true
		}
	}

	return AliasEntry{}, false
}

func IsBuiltinURI(uri string) bool {
	return !strings.Contains(uri, "://")
}

func DirFromURI(uri string) string {
	if IsBuiltinURI(uri) {
		return ""
	}

	body := uri
	if idx := strings.Index(body, "://"); idx >= 0 {
		body = body[idx+len("://"):]
	}
	body = strings.TrimLeft(body, "/")
	if idx := strings.IndexByte(body, '?'); idx >= 0 {
		body = body[:idx]
	}
	if idx := strings.LastIndexByte(body, '/'); idx >= 0 {
		return body[idx+1:]
	}

	return body
}

func BuiltinAliases(entries []AliasEntry) []string {
	var aliases []string
	for _, entry := range entries {
		if IsBuiltinURI(entry.URI) {
			aliases = append(aliases, entry.Alias)
		}
	}
	sort.Strings(aliases)

	return aliases
}
