// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

// Package slc implements the launcher-owned script language container component:
// parsing the official SLC catalog, resolving a user-supplied alias to a concrete
// container image, and enforcing the alias-uniqueness rule that lets multiple SLCs
// coexist. It is deliberately free of deployment/backend dependencies so the logic
// is pure and unit-testable; the deploy layer bridges it to launcher state and the
// local runtime.
package slc

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

// slcMountRoot is the directory the database scans for built-in script language
// containers. Each SLC is mounted at slcMountRoot/<flavor>.
const slcMountRoot = "/exa/slc"

// Catalog is the parsed representation of slc-catalog.yaml.
type Catalog struct {
	Registry string `yaml:"registry"`
	//nolint:tagliatelle // YAML schema uses snake_case field names.
	TagTemplate   string                  `yaml:"tag_template"`
	Architectures map[string]Architecture `yaml:"architectures"`
}

// Architecture holds the SLCs available for one container architecture.
type Architecture struct {
	//nolint:tagliatelle // YAML schema uses snake_case field names.
	DefaultVersion string             `yaml:"default_version"`
	Versions       map[string]Version `yaml:"versions"`
}

// Version groups the language flavors shipped in one script-languages-release version.
type Version struct {
	Languages map[string]Language `yaml:"languages"`
}

// Language describes a single flavor: its image flavor token, content hash, and the
// aliases it declares in build_info/language_definitions.json.
type Language struct {
	Flavor  string   `yaml:"flavor"`
	Hash    string   `yaml:"hash"`
	Aliases []string `yaml:"aliases"`
}

// Entry is a fully resolved SLC ready to be mounted.
type Entry struct {
	// Language is the catalog key (e.g. "python").
	Language string
	// Flavor is the image flavor token (e.g. "python-3.12").
	Flavor string
	// Version is the catalog version the entry was resolved from.
	Version string
	// Image is the full container image reference (registry:tag).
	Image string
	// Target is the mount destination inside the database container.
	Target string
	// Aliases are all aliases the SLC declares.
	Aliases []string
}

// ErrArchitectureUnsupported reports that the catalog has no SLCs for a given
// architecture. Callers that must degrade gracefully (e.g. `slc list`) can test for it
// with errors.Is; operations that require a concrete SLC (install/update) let it surface.
var ErrArchitectureUnsupported = errors.New("architecture is not supported")

// UnknownAliasError reports an alias that matches no catalog entry.
type UnknownAliasError struct {
	Alias        string
	ValidAliases []string
}

func (e *UnknownAliasError) Error() string {
	if len(e.ValidAliases) == 0 {
		return fmt.Sprintf("unknown SLC alias %q", e.Alias)
	}

	return fmt.Sprintf(
		"unknown SLC alias %q; available aliases: %s",
		e.Alias,
		strings.Join(e.ValidAliases, ", "),
	)
}

// Load parses an SLC catalog from its YAML representation.
func Load(data []byte) (*Catalog, error) {
	var catalog Catalog
	if err := yaml.Unmarshal(data, &catalog); err != nil {
		return nil, fmt.Errorf("failed to parse SLC catalog: %w", err)
	}

	if strings.TrimSpace(catalog.Registry) == "" {
		return nil, errors.New("SLC catalog is missing 'registry'")
	}
	if strings.TrimSpace(catalog.TagTemplate) == "" {
		return nil, errors.New("SLC catalog is missing 'tag_template'")
	}

	return &catalog, nil
}

// Resolve maps a user-supplied alias (matched case-insensitively) to a concrete SLC in
// the architecture's default version.
func (c *Catalog) Resolve(alias, goarch string) (Entry, error) {
	normalized := strings.TrimSpace(alias)
	if normalized == "" {
		return Entry{}, errors.New("no SLC alias provided")
	}

	entries, err := c.entries(goarch)
	if err != nil {
		return Entry{}, err
	}

	for _, entry := range entries {
		for _, candidate := range entry.Aliases {
			if strings.EqualFold(candidate, normalized) {
				return entry, nil
			}
		}
	}

	return Entry{}, &UnknownAliasError{
		Alias:        normalized,
		ValidAliases: aliasList(entries),
	}
}

// List returns every SLC available in the architecture's default version, sorted by
// language for a stable presentation.
func (c *Catalog) List(goarch string) ([]Entry, error) {
	return c.entries(goarch)
}

func (c *Catalog) entries(goarch string) ([]Entry, error) {
	arch, ok := c.Architectures[goarch]
	if !ok {
		return nil, fmt.Errorf(
			"no SLCs available for architecture %q: %w",
			goarch,
			ErrArchitectureUnsupported,
		)
	}

	version := strings.TrimSpace(arch.DefaultVersion)
	if version == "" {
		return nil, fmt.Errorf("no default_version set for architecture %q", goarch)
	}

	langs, ok := arch.Versions[version]
	if !ok {
		return nil, fmt.Errorf("default_version %q not found for architecture %q", version, goarch)
	}

	names := make([]string, 0, len(langs.Languages))
	for name := range langs.Languages {
		names = append(names, name)
	}
	sort.Strings(names)

	entries := make([]Entry, 0, len(names))
	for _, name := range names {
		lang := langs.Languages[name]
		entries = append(entries, Entry{
			Language: name,
			Flavor:   lang.Flavor,
			Version:  version,
			Image:    c.imageRef(lang.Flavor, goarch, lang.Hash),
			Target:   targetDir(lang.Flavor),
			Aliases:  lang.Aliases,
		})
	}

	return entries, nil
}

func (c *Catalog) imageRef(flavor, goarch, hash string) string {
	tag := c.TagTemplate
	tag = strings.ReplaceAll(tag, "{flavor}", flavor)
	tag = strings.ReplaceAll(tag, "{arch}", goarch)
	tag = strings.ReplaceAll(tag, "{hash}", hash)

	return c.Registry + ":" + tag
}

func targetDir(flavor string) string {
	return slcMountRoot + "/" + flavor
}

func aliasList(entries []Entry) []string {
	seen := make(map[string]struct{})
	var all []string
	for _, entry := range entries {
		for _, alias := range entry.Aliases {
			if _, ok := seen[alias]; ok {
				continue
			}
			seen[alias] = struct{}{}
			all = append(all, alias)
		}
	}
	sort.Strings(all)

	return all
}
