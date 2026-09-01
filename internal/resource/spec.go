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

type ResourceSpec map[string]ResourceDefinition

type Specification struct {
	definitions ResourceSpec
	platform    Platform
	parent      *Specification
}

func NewSpecification(definitions ResourceSpec, platform Platform) *Specification {
	return &Specification{definitions: definitions, platform: platform}
}

func (s *Specification) Lookup(resourceID string) (Descriptor, error) {
	definition, ok := s.definitions[resourceID]
	if !ok {
		if s.parent != nil {
			return s.parent.Lookup(resourceID)
		}

		return Descriptor{}, fmt.Errorf(
			"%w: unknown resource %q", ErrUnknownMember, resourceID,
		)
	}

	return definition.descriptor(s.platform.GOOS, s.platform.GOARCH)
}

func (s *Specification) List(prefix string) []string {
	ids := make([]string, 0, len(s.definitions))
	seen := map[string]struct{}{}
	for resourceID := range s.definitions {
		if strings.HasPrefix(resourceID, prefix) {
			ids = append(ids, resourceID)
			seen[resourceID] = struct{}{}
		}
	}
	if s.parent != nil {
		for _, resourceID := range s.parent.List(prefix) {
			if _, ok := seen[resourceID]; !ok {
				ids = append(ids, resourceID)
			}
		}
	}
	slices.Sort(ids)

	return ids
}

//nolint:golines // golines and tagalign disagree on struct tag alignment here; tagalign wins.
type ResourceDefinition struct {
	Extract  bool                    `json:"extract,omitempty" yaml:"extract,omitempty"`
	Embed    EmbedMode               `json:"embed,omitempty"   yaml:"embed,omitempty"`
	Artifact map[string]ArtifactSpec `json:"artifact"          yaml:"artifact"`
	// A non-empty pattern expands during generation and is omitted from resolved specs.
	Glob string `json:"glob,omitempty" yaml:"glob,omitempty"`
}

// EmbedMode: pointer unmarshalling and value marshalling support non-addressable map values.
//
//nolint:recvcheck
type EmbedMode int

const (
	EmbedNever EmbedMode = iota
	EmbedDefault
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

type ArtifactSpec struct {
	URL    string `yaml:"url"`
	Ref    string `yaml:"ref,omitempty"`
	Sha256 string `yaml:"sha256,omitempty"`
	// The suffix selects the extractor even when the source is a bare directory.
	//nolint:tagliatelle // YAML schema uses snake_case field names.
	DownloadPath string `yaml:"download_path,omitempty"`
	Subpath      string `yaml:"subpath,omitempty"`
}

func (a ArtifactSpec) Locator() Locator {
	if strings.TrimSpace(a.Ref) != "" {
		return Locator{URL: a.URL, Ref: a.Ref}
	}
	rawURL, ref := splitGitRef(a.URL)

	return Locator{URL: rawURL, Ref: ref}
}

func isDigestPinnedImage(loc Locator) bool {
	if !(&DockerSource{}).Handles(loc) {
		return false
	}
	probe, err := (&DockerSource{}).Probe(context.Background(), loc)

	return err == nil && probe.Identity != ""
}

const anyPlatformKey = "any"

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

func (d ResourceDefinition) descriptor(goos, goarch string) (Descriptor, error) {
	artifact, err := d.Resolve(goos, goarch)
	if err != nil {
		return Descriptor{}, err
	}

	return Descriptor{
		Locator:      artifact.Locator(),
		Sha256:       artifact.Sha256,
		Extract:      d.Extract,
		Subpath:      artifact.Subpath,
		DownloadPath: artifact.DownloadPath,
	}, nil
}

func platformKey(goos, goarch string) string {
	return goos + "/" + goarch
}

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
		// Local sources inherit trust from the repository that contains them.
	case isDigestPinnedImage(locator):
		// A digest already identifies the image content.
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
