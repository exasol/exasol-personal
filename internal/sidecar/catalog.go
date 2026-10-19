// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package sidecar

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
)

type Resolver interface {
	// Embedded documents and runtime entries answer this the same way, so a
	// selection resolves the same way whatever produced it.
	Entries() map[string]Entry
	Resolve(name, architecture string) (Container, error)
}

// Catalogs resolves a name against its members in order, so entries assembled
// for one command take precedence over the launcher's embedded templates.
type Catalogs []Resolver

func LoadCatalog(data []byte) (*Catalog, error) {
	var catalog Catalog
	if err := decode(data, &catalog); err != nil {
		return nil, err
	}
	if catalog.Version != 1 || catalog.Sidecars == nil {
		return nil, errors.New("document: version must be 1 and sidecars must be present")
	}

	return NewCatalog(catalog.Sidecars)
}

// NewCatalog validates entries assembled at runtime against the same rules as
// an embedded catalog document.
func NewCatalog(entries map[string]Entry) (*Catalog, error) {
	catalog := Catalog{Version: 1, Sidecars: map[string]Entry{}}
	for _, name := range slices.Sorted(maps.Keys(entries)) {
		entry := entries[name].Clone()
		location := "sidecars." + name
		if entry.Container.Name == "" {
			entry.Container.Name = name
		}
		if entry.Container.Name != name {
			return nil, fmt.Errorf("%s.container.name: must match catalog key", location)
		}
		if strings.TrimSpace(entry.Description) == "" || len(entry.Architectures) == 0 {
			return nil, fmt.Errorf("%s: description and architectures are required", location)
		}
		for _, arch := range entry.Architectures {
			if arch != "amd64" && arch != "arm64" {
				return nil, fmt.Errorf(
					"%s.architectures: unsupported architecture %q",
					location,
					arch,
				)
			}
		}
		if err := entry.Container.Validate(location + ".container"); err != nil {
			return nil, err
		}
		entry.Container = entry.Container.Defaults()
		catalog.Sidecars[name] = entry
	}

	return &catalog, nil
}

func (catalog *Catalog) Entries() map[string]Entry { return catalog.Sidecars }

func (catalog *Catalog) Resolve(name, architecture string) (Container, error) {
	return resolveEntry(catalog.Sidecars, name, architecture)
}

func (catalogs Catalogs) Entries() map[string]Entry {
	entries := map[string]Entry{}
	for _, catalog := range slices.Backward(catalogs) {
		maps.Copy(entries, catalog.Entries())
	}

	return entries
}

func (catalogs Catalogs) Resolve(name, architecture string) (Container, error) {
	// Precedence is per name and architecture, so an earlier catalog that
	// carries the name for other architectures does not shadow a later one.
	for _, catalog := range catalogs {
		if container, err := catalog.Resolve(name, architecture); err == nil {
			return container, nil
		}
	}
	// No member supplies it, so report the choices all of them offer together.
	return resolveEntry(catalogs.Entries(), name, architecture)
}

func resolveEntry(entries map[string]Entry, name, architecture string) (Container, error) {
	entry, exists := entries[name]
	if exists && slices.Contains(entry.Architectures, architecture) {
		// Callers may edit a selected template without changing subsequent selections.
		return entry.Container.Clone(), nil
	}
	choices := make([]string, 0, len(entries))
	for key, candidate := range entries {
		if slices.Contains(candidate.Architectures, architecture) {
			choices = append(choices, key)
		}
	}
	slices.Sort(choices)

	if !exists {
		return Container{}, fmt.Errorf("sidecar %q is not in the catalog; supported choices: %s",
			name, strings.Join(choices, ", "))
	}

	return Container{}, fmt.Errorf("sidecar %q is not available for %s; supported choices: %s",
		name, architecture, strings.Join(choices, ", "))
}
