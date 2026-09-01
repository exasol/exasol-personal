// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package resource

import (
	"context"
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
	Extract bool `json:"extract,omitempty" yaml:"extract,omitempty"`
	// Embed is a build directive: it selects what a build embeds and never
	// appears in a resolved specification.
	Embed    EmbedMode               `json:"embed,omitempty" yaml:"embed,omitempty"`
	Artifact map[string]ArtifactSpec `json:"artifact"        yaml:"artifact"`
	// Glob is a build-time pattern matched against the resource's own resolved
	// content. Each match becomes an independently addressable resource named
	// "<resource>/<match>", and the group itself does not reach a build.
	Glob string `json:"glob,omitempty" yaml:"glob,omitempty"`
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
	URL string `yaml:"url"`
	// Ref selects a revision within the source, such as a branch, tag, or
	// commit for a git repository.
	Ref    string `yaml:"ref,omitempty"`
	Sha256 string `yaml:"sha256,omitempty"`
	// DownloadPath's suffix picks the extractor (see extractors), so it must
	// look like an archive even when the source itself is a bare directory.
	//nolint:tagliatelle // YAML schema uses snake_case field names.
	DownloadPath string `yaml:"download_path,omitempty"`
	Subpath      string `yaml:"subpath,omitempty"`
}

// Locator resolves the artifact's location, taking a Git revision from Ref
// when declared and from the URL's own "@ref" suffix otherwise.
func (a ArtifactSpec) Locator() Locator {
	if strings.TrimSpace(a.Ref) != "" {
		return Locator{URL: a.URL, Ref: a.Ref}
	}
	rawURL, ref := splitGitRef(a.URL)

	return Locator{URL: rawURL, Ref: ref}
}

// isDigestPinnedImage reports whether loc is a container image reference that
// names its content by digest.
func isDigestPinnedImage(loc Locator) bool {
	if !(&DockerSource{}).Handles(loc) {
		return false
	}
	probe, err := (&DockerSource{}).Probe(context.Background(), loc)

	return err == nil && probe.Identity != ""
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
	if d.Glob != "" && strings.TrimSpace(d.Glob) == "" {
		return fmt.Errorf("resource %q declares a blank glob pattern", resourceID)
	}
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
	locator := a.Locator()

	switch {
	case (&GitSource{}).Handles(locator):
		if strings.TrimSpace(a.Sha256) != "" {
			return fmt.Errorf(
				"resource %q artifact %q must not define sha256 for a git source"+
					" (commit hash is used instead)",
				ctx.resourceID,
				ctx.variant,
			)
		}
	case (FileSource{}).Handles(locator):
		// Local (file://, or bare local path) sources are first-party content whose
		// integrity comes from being part of the same versioned repository commit,
		// not from a hand-authored checksum, so a checksum is optional for them.
	case isDigestPinnedImage(locator):
		// A digest names the image content itself, so it is already the checksum.
	default:
		if strings.TrimSpace(a.Sha256) == "" {
			return fmt.Errorf(
				"resource %q artifact %q must define sha256",
				ctx.resourceID,
				ctx.variant,
			)
		}
	}

	return nil
}
