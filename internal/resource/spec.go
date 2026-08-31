// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package resource

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// ResourceSpec contains all embedded runtime resources keyed by logical resource ID.
type ResourceSpec map[string]ResourceDefinition

// ResourceDefinition describes how to fetch and materialize a resource.
//
//nolint:golines // golines and tagalign disagree on struct tag alignment here; tagalign wins.
type ResourceDefinition struct {
	Extract  bool                    `json:"extract"  yaml:"extract"`
	Embed    EmbedMode               `json:"embed"    yaml:"embed"`
	Artifact map[string]ArtifactSpec `json:"artifact" yaml:"artifact"`
	// Glob marks resource_path as a pattern to match within the artifact's
	// resolved content, instead of a literal subpath.
	Glob bool `json:"glob,omitempty" yaml:"glob,omitempty"`
}

// EmbedMode declares whether a resource's data is embedded into the compiled
// binary, and whether that still happens for a build that otherwise skips
// real embedding for speed (SKIP_EMBED=true, used by lint/test builds that
// never look at the bytes). It parses from YAML/JSON as one of the values
// false, true, or "always".
// Unmarshal* needs a pointer receiver to mutate; Marshal* deliberately stays
// on a value receiver so it still works on a non-addressable EmbedMode, such
// as a ResourceDefinition read from a map value.
//
//nolint:recvcheck
type EmbedMode int

const (
	// EmbedNever fetches this resource at runtime; it is never embedded.
	EmbedNever EmbedMode = iota
	// EmbedDefault embeds this resource in real builds, but is skipped under
	// SKIP_EMBED=true.
	EmbedDefault
	// EmbedAlways embeds this resource even under SKIP_EMBED=true. Reserved
	// for small, locally-sourced resources that cost nothing to embed.
	EmbedAlways
)

func embedModeFromValue(raw any) (EmbedMode, error) {
	switch value := raw.(type) {
	case bool:
		if value {
			return EmbedDefault, nil
		}

		return EmbedNever, nil
	case string:
		if value == "always" {
			return EmbedAlways, nil
		}
	default:
	}

	return EmbedNever, fmt.Errorf(
		"invalid embed value %#v: must be true, false, or %q", raw, "always",
	)
}

func (m *EmbedMode) UnmarshalYAML(node *yaml.Node) error {
	var raw any
	if err := node.Decode(&raw); err != nil {
		return err
	}
	mode, err := embedModeFromValue(raw)
	if err != nil {
		return err
	}
	*m = mode

	return nil
}

func (m EmbedMode) MarshalYAML() (any, error) {
	return m.embedValue()
}

func (m *EmbedMode) UnmarshalJSON(data []byte) error {
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	mode, err := embedModeFromValue(raw)
	if err != nil {
		return err
	}
	*m = mode

	return nil
}

func (m EmbedMode) MarshalJSON() ([]byte, error) {
	value, err := m.embedValue()
	if err != nil {
		return nil, err
	}

	return json.Marshal(value)
}

func (m EmbedMode) embedValue() (any, error) {
	switch m {
	case EmbedNever:
		return false, nil
	case EmbedDefault:
		return true, nil
	case EmbedAlways:
		return "always", nil
	default:
		return nil, fmt.Errorf("invalid embed mode %d", m)
	}
}

// ArtifactSpec describes one downloadable artifact for a specific platform.
type ArtifactSpec struct {
	URL    string `yaml:"url"`
	Sha256 string `yaml:"sha256"`
	// DownloadPath's suffix picks the extractor (see extractors), so it must
	// look like an archive even when the source itself is a bare directory.
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
			slices.Sort(keys)

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
	slices.Sort(keys)
	for _, key := range keys {
		artifact := d.Artifact[key]
		if err := artifact.validate(artifactValidationContext{
			resourceID: resourceID,
			variant:    key,
			extract:    d.Extract,
			glob:       d.Glob,
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
	glob       bool
}

func (a ArtifactSpec) validate(ctx artifactValidationContext) error {
	if strings.TrimSpace(a.URL) == "" {
		return fmt.Errorf("resource %q artifact %q must define url", ctx.resourceID, ctx.variant)
	}
	// ArtifactSpec.URL may carry an @ref suffix; strip it before classification.
	gitRepoURL, _ := ParseGitURL(a.URL)

	switch {
	case IsGitSourceURL(gitRepoURL):
		if strings.TrimSpace(a.Sha256) != "" {
			return fmt.Errorf(
				"resource %q artifact %q must not define sha256 for a git source"+
					" (commit hash is used instead)",
				ctx.resourceID,
				ctx.variant,
			)
		}
	case (FileSource{}).CanFetch(a.URL):
		// Local (file://, or bare local path) sources are first-party content whose
		// integrity comes from being part of the same versioned repository commit,
		// not from a hand-authored checksum, so a checksum is optional for them.
	default:
		if strings.TrimSpace(a.Sha256) == "" {
			return fmt.Errorf(
				"resource %q artifact %q must define sha256",
				ctx.resourceID,
				ctx.variant,
			)
		}
	}

	if ctx.glob {
		return validateGlobResourcePath(a, ctx)
	}

	return nil
}

// A git checkout is already a directory, so a glob template with a git
// source must not declare extract: true. Any other non-local source must,
// since a git checkout or an extracted archive is the only way to have a
// directory tree to glob within.
func validateGlobResourcePath(artifact ArtifactSpec, ctx artifactValidationContext) error {
	if strings.TrimSpace(artifact.ResourcePath) == "" {
		return fmt.Errorf(
			"resource %q artifact %q must define resource_path with a glob pattern",
			ctx.resourceID,
			ctx.variant,
		)
	}
	// ArtifactSpec.URL may carry an @ref suffix; strip it before classification.
	gitRepoURL, _ := ParseGitURL(artifact.URL)
	if IsGitSourceURL(gitRepoURL) {
		if ctx.extract {
			return fmt.Errorf(
				"resource %q artifact %q must not declare extract: true for a git source"+
					" (a checkout is already a directory)",
				ctx.resourceID,
				ctx.variant,
			)
		}

		return nil
	}
	if (FileSource{}).CanFetch(artifact.URL) {
		return nil
	}
	if !ctx.extract {
		return fmt.Errorf(
			"resource %q artifact %q must declare extract: true to use a glob resource_path",
			ctx.resourceID,
			ctx.variant,
		)
	}

	return nil
}
