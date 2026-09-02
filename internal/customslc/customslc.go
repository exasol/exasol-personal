// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

// Package customslc is kept free of deployment/backend dependencies so its SCRIPT_LANGUAGES
// and activation-URI logic stays pure and unit-testable.
package customslc

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

const aliasURIParts = 2

const clientRelPath = "exaudf/exaudfclient"

const builtinBucketPath = "__builtin__/slc"

var languageIdentifierPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

type AliasEntry struct {
	Alias string
	URI   string
}

type Language string

func NormalizeLanguage(raw string) (Language, error) {
	language := strings.ToLower(strings.TrimSpace(raw))
	if !languageIdentifierPattern.MatchString(language) {
		return "", fmt.Errorf(
			"invalid language identifier %q: must start with a letter or digit and use only "+
				"ASCII letters, digits, dots, hyphens, and underscores", raw,
		)
	}

	return Language(language), nil
}

// NormalizeAlias upper-cases aliases to match database-reported identifiers.
func NormalizeAlias(alias string) string {
	return strings.ToUpper(strings.TrimSpace(alias))
}

func BuildActivationURI(dir string, language Language) string {
	return fmt.Sprintf(
		"localzmq+protobuf:///%s/%s?lang=%s#/%s",
		builtinBucketPath, dir, language, clientRelPath,
	)
}

// ParseScriptLanguages skips tokens without an '=' rather than guessing at their meaning.
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

// SetAlias preserves every other entry, including builtins, through a read-merge-write update.
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
	slices.Sort(aliases)

	return aliases
}
