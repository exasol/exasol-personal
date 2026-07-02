// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package runtimeartifacts

import (
	"fmt"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

// ResourceSpec contains all embedded runtime resources keyed by logical resource ID.
type ResourceSpec map[string]ResourceDefinition

// ResourceDefinition describes how to fetch and materialize a resource.
type ResourceDefinition struct {
	Extract  bool                    `json:"extract"  yaml:"extract"`
	Artifact map[string]ArtifactSpec `json:"artifact" yaml:"artifact"`
}

// ArtifactSpec describes one downloadable artifact for a specific platform.
type ArtifactSpec struct {
	URL    string `yaml:"url"`
	Sha256 string `yaml:"sha256"`
	//nolint:tagliatelle // YAML schema uses snake_case field names.
	DownloadPath string `yaml:"download_path,omitempty"`
	//nolint:tagliatelle // YAML schema uses snake_case field names.
	ResourcePath string `yaml:"resource_path,omitempty"`
}

const anyPlatformKey = "any"

// Resolve returns the artifact for the requested platform.
func (d ResourceDefinition) Resolve(goos, goarch string) (ArtifactSpec, error) {
	key := platformKey(goos, goarch)
	artifact, ok := d.Artifact[key]
	if !ok {
		artifact, ok = d.Artifact[anyPlatformKey]
		if !ok {
			keys := make([]string, 0, len(d.Artifact))
			for candidate := range d.Artifact {
				keys = append(keys, candidate)
			}
			sort.Strings(keys)

			return ArtifactSpec{}, fmt.Errorf(
				"no artifact for platform %s in resource; available variants: %s",
				key,
				strings.Join(keys, ", "),
			)
		}
	}

	return artifact, nil
}

func platformKey(goos, goarch string) string {
	return goos + "/" + goarch
}

// ParseSpec parses an embedded resource specification from YAML.
func ParseSpec(raw []byte) (ResourceSpec, error) {
	var spec ResourceSpec
	if err := yaml.Unmarshal(raw, &spec); err != nil {
		return nil, err
	}

	for resourceID, def := range spec {
		if err := def.validate(resourceID); err != nil {
			return nil, err
		}
	}

	return spec, nil
}

func (d ResourceDefinition) validate(resourceID string) error {
	if len(d.Artifact) == 0 {
		return fmt.Errorf(
			"resource %q must define a platform-specific artifact",
			resourceID,
		)
	}

	keys := make([]string, 0, len(d.Artifact))
	for key := range d.Artifact {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		artifact := d.Artifact[key]
		if err := artifact.validate(artifactValidationContext{
			resourceID: resourceID,
			variant:    key,
			extract:    d.Extract,
		}); err != nil {
			return err
		}
		if key != anyPlatformKey && !strings.Contains(key, "/") {
			return fmt.Errorf(
				"resource %q uses invalid platform key %q; expected GOOS/GOARCH or %q",
				resourceID,
				key,
				anyPlatformKey,
			)
		}
	}

	return nil
}

type artifactValidationContext struct {
	resourceID string
	variant    string
	extract    bool
}

func (a ArtifactSpec) validate(ctx artifactValidationContext) error {
	if strings.TrimSpace(a.URL) == "" {
		return fmt.Errorf("resource %q artifact %q must define url", ctx.resourceID, ctx.variant)
	}
	if IsGitSourceURL(a.URL) {
		if strings.TrimSpace(a.Sha256) != "" {
			return fmt.Errorf(
				"resource %q artifact %q must not define sha256 for a git source"+
					" (commit hash is used instead)",
				ctx.resourceID,
				ctx.variant,
			)
		}
	} else {
		if strings.TrimSpace(a.Sha256) == "" {
			return fmt.Errorf(
				"resource %q artifact %q must define sha256",
				ctx.resourceID,
				ctx.variant,
			)
		}
	}
	if !ctx.extract && strings.TrimSpace(a.ResourcePath) != "" {
		return fmt.Errorf(
			"resource %q artifact %q must not define resource_path without archive extraction",
			ctx.resourceID,
			ctx.variant,
		)
	}

	return nil
}
