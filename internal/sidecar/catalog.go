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

// Resolver supplies the templates a deployment can enable. Embedded catalog
// documents and entries assembled at runtime answer the same contract, so a
// selection resolves the same way whatever produced it.
type Resolver interface {
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
	for _, catalog := range catalogs {
		if container, err := catalog.Resolve(name, architecture); err == nil {
			return container, nil
		}
	}

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

	return Container{}, fmt.Errorf("sidecar %q is unavailable for %s; supported choices: %s",
		name, architecture, strings.Join(choices, ", "))
}

func Parse(data []byte) (Document, error) {
	var document Document
	if err := decode(data, &document); err != nil {
		return Document{}, err
	}
	if document.Version != 1 || document.Containers == nil {
		return Document{}, errors.New("document: version must be 1 and containers must be present")
	}
	names := make(map[string]bool)
	for index, container := range document.Containers {
		location := fmt.Sprintf("containers[%d]", index)
		if names[container.Name] {
			return Document{}, fmt.Errorf("%s.name: duplicate container name", location)
		}
		names[container.Name] = true
		if err := container.Validate(location); err != nil {
			return Document{}, err
		}
	}

	return document, nil
}

func (entry Entry) Clone() Entry {
	entry.Architectures = slices.Clone(entry.Architectures)
	entry.Container = entry.Container.Clone()

	return entry
}

func (container Container) Clone() Container {
	container.Command = slices.Clone(container.Command)
	container.Args = slices.Clone(container.Args)
	container.Ports = slices.Clone(container.Ports)
	container.Env = slices.Clone(container.Env)
	for index, env := range container.Env {
		if env.Value != nil {
			value := *env.Value
			container.Env[index].Value = &value
		}
		if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil {
			reference := *env.ValueFrom.SecretKeyRef
			container.Env[index].ValueFrom = &EnvSource{SecretKeyRef: &reference}
		}
	}

	return container
}

func (container Container) Defaults() Container {
	container = container.Clone()
	if container.RestartPolicy == "" {
		container.RestartPolicy = OnFailure
	}
	if container.ImagePullPolicy == "" {
		container.ImagePullPolicy = IfNotPresent
		image := container.Image
		if !strings.Contains(image, "@") {
			last := image[strings.LastIndex(image, "/")+1:]
			if !strings.Contains(last, ":") || strings.HasSuffix(last, ":latest") {
				container.ImagePullPolicy = Always
			}
		}
	}
	for index := range container.Ports {
		port := &container.Ports[index]
		if port.Protocol == "" {
			port.Protocol = "TCP"
		}
		if port.HostPort > 0 && port.HostIP == "" {
			port.HostIP = "127.0.0.1"
		}
	}

	return container
}
