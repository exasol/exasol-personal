// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package sidecar_test

import (
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/assets/resources"
	"github.com/exasol/exasol-personal/internal/sidecar"
	"github.com/stretchr/testify/require"
)

const catalogYAML = `version: 1
sidecars:
  example:
    description: Example service
    architectures: [amd64, arm64]
    container:
      image: docker.io/library/caddy:2-alpine
      env:
        - name: MARKER
          value: original
`

func TestCatalogDefaultsAndIsolatesSelections(t *testing.T) {
	t.Parallel()
	// Given
	catalog, err := sidecar.LoadCatalog([]byte(catalogYAML))
	require.NoError(t, err)
	// When
	selected, err := catalog.Resolve("example", "arm64")
	require.NoError(t, err)
	*selected.Env[0].Value = "edited"
	again, err := catalog.Resolve("example", "amd64")
	require.NoError(t, err)
	// Then
	if again.Name != "example" || again.RestartPolicy != "OnFailure" ||
		again.ImagePullPolicy != "IfNotPresent" || *again.Env[0].Value != "original" {
		t.Fatalf("unexpected selection: %+v", again)
	}
}

func TestCatalogRejectsUnavailableSelection(t *testing.T) {
	t.Parallel()
	// Given
	catalog, err := sidecar.LoadCatalog([]byte(catalogYAML))
	require.NoError(t, err)
	// When
	_, unknown := catalog.Resolve("missing", "amd64")
	_, incompatible := catalog.Resolve("example", "s390x")
	// Then
	if unknown == nil || !strings.Contains(unknown.Error(), "example") || incompatible == nil {
		t.Fatalf("expected selection errors, got %v and %v", unknown, incompatible)
	}
}

func TestEmbeddedSidecarCatalog(t *testing.T) {
	t.Parallel()
	// Given
	data := resources.SidecarCatalogYAML
	// When
	_, err := sidecar.LoadCatalog(data)
	// Then
	require.NoError(t, err)
}

func TestCatalogRejectsInvalidMetadata(t *testing.T) {
	t.Parallel()
	for _, fixture := range []struct{ name, old, replacement, location string }{
		{"version", "version: 1", "version: 2", "document"},
		{"reserved", "example:", "database:", "sidecars.database.container.name"},
		{"mismatch", "image:", "name: another\n      image:", "container.name"},
		{"architecture", "[amd64, arm64]", "[s390x]", "architectures"},
		{"description", "Example service", "''", "sidecars.example"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			// Given
			data := strings.Replace(catalogYAML, fixture.old, fixture.replacement, 1)
			// When
			_, err := sidecar.LoadCatalog([]byte(data))
			// Then
			if err == nil || !strings.Contains(err.Error(), fixture.location) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestSidecarInvalidConfigurationLocation(t *testing.T) {
	t.Parallel()
	for _, fixture := range []struct{ name, container, location string }{
		{"unknown", "image: caddy\n    resources: {}", "containers[0].resources"},
		{
			"nested", "image: caddy\n    env: [{name: X, valueFrom: {fieldRef: {}}}]",
			"env[0].valueFrom.fieldRef",
		},
		{"number", "image: 123", "containers[0].image"},
		{"null", "image: null", "containers[0].image"},
		{"image", "image: '-invalid'", "containers[0].image"},
		{"pull", "image: caddy\n    imagePullPolicy: Sometimes", "imagePullPolicy"},
		{"restart", "image: caddy\n    restartPolicy: Sometimes", "restartPolicy"},
		{"duplicate", "image: caddy\n    image: caddy", "containers[0].image"},
		{"env_duplicate", "image: caddy\n    env: [{name: X}, {name: X}]", "env[1].name"},
		{"env_conflict", "image: caddy\n    env: [{name: X, value: '', " +
			"valueFrom: {secretKeyRef: {name: x, key: y}}}]", "env[0]"},
		{
			"env_source", "image: caddy\n    env: [{name: X, valueFrom: {}}]",
			"env[0].valueFrom.secretKeyRef",
		},
		{"port", "image: caddy\n    ports: [{containerPort: 0}]", "ports[0].containerPort"},
		{
			"host_port", "image: caddy\n    ports: [{containerPort: 80, hostPort: 65536}]",
			"ports[0].hostPort",
		},
		{
			"protocol", "image: caddy\n    ports: [{containerPort: 80, protocol: UDP}]",
			"ports[0].protocol",
		},
		{
			"duplicate_name", "image: caddy\n  - name: example\n    image: caddy",
			"containers[1].name",
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			// Given
			data := "version: 1\ncontainers:\n  - name: example\n    " + fixture.container + "\n"
			// When
			_, err := sidecar.Parse([]byte(data))
			// Then
			if err == nil || !strings.Contains(err.Error(), fixture.location) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestDocumentBoundaries(t *testing.T) {
	t.Parallel()
	for _, data := range []string{
		"", "[", "version: 1", "version: 1\ncontainers: null",
		"version: 1\ncontainers: []\n---\nversion: 1",
	} {
		t.Run(data, func(t *testing.T) {
			t.Parallel()
			// Given
			input := []byte(data)
			// When
			_, err := sidecar.Parse(input)
			// Then
			require.Error(t, err, "expected an error")
		})
	}
}

func TestImagePolicyDefaults(t *testing.T) {
	t.Parallel()
	for _, fixture := range []struct{ image, want string }{
		{"caddy", "Always"},
		{"caddy:latest", "Always"},
		{"caddy:2", "IfNotPresent"},
		{"localhost:5000/caddy", "Always"},
		{"caddy@sha256:abcd", "IfNotPresent"},
	} {
		t.Run(fixture.image, func(t *testing.T) {
			t.Parallel()
			// Given
			container := sidecar.Container{Image: fixture.image}
			// When
			actual := container.Defaults()
			// Then
			require.Equal(t, fixture.want, actual.ImagePullPolicy)
		})
	}
}

func TestSidecarLoopbackPublicationValidation(t *testing.T) {
	t.Parallel()
	for _, address := range []string{"0.0.0.0", "::", "192.0.2.1", "localhost"} {
		t.Run(address, func(t *testing.T) {
			t.Parallel()
			// Given
			port := sidecar.Port{ContainerPort: 8080, HostPort: 18080, HostIP: address}
			// When
			err := port.Validate("containers[0].ports[0]")
			// Then
			if err == nil || !strings.Contains(err.Error(), "ports[0].hostIP") ||
				!strings.Contains(err.Error(), "loopback") {
				t.Fatalf("unexpected validation: %v", err)
			}
		})
	}
}

func runtimeCatalog(t *testing.T, image string) *sidecar.Catalog {
	t.Helper()
	catalog, err := sidecar.NewCatalog(map[string]sidecar.Entry{
		"example": {
			Description:   "Runtime service",
			Architectures: []string{"amd64"},
			Container:     sidecar.Container{Image: image},
		},
	})
	require.NoError(t, err)

	return catalog
}

func TestRuntimeEntriesResolveAheadOfTheCatalog(t *testing.T) {
	t.Parallel()
	// Given
	embedded, err := sidecar.LoadCatalog([]byte(catalogYAML))
	require.NoError(t, err)
	catalogs := sidecar.Catalogs{runtimeCatalog(t, "docker.io/library/redis:8"), embedded}
	// When
	selected, err := catalogs.Resolve("example", "amd64")
	// Then
	require.NoError(t, err)
	require.Equal(t, "docker.io/library/redis:8", selected.Image)
	require.Equal(t, "example", selected.Name)
	require.Equal(t, "Runtime service", catalogs.Entries()["example"].Description)
}

func TestRuntimeEntriesKeepUnsupportedArchitecturesOnTheCatalog(t *testing.T) {
	t.Parallel()
	// Given
	embedded, err := sidecar.LoadCatalog([]byte(catalogYAML))
	require.NoError(t, err)
	catalogs := sidecar.Catalogs{runtimeCatalog(t, "docker.io/library/redis:8"), embedded}
	// When
	selected, err := catalogs.Resolve("example", "arm64")
	// Then
	require.NoError(t, err)
	require.Equal(t, "docker.io/library/caddy:2-alpine", selected.Image)
}

func TestRuntimeEntriesFollowCatalogValidation(t *testing.T) {
	t.Parallel()
	for name, entry := range map[string]sidecar.Entry{
		"missing description": {
			Architectures: []string{"amd64"},
			Container:     sidecar.Container{Image: "caddy:2"},
		},
		"missing architectures": {
			Description: "Runtime service",
			Container:   sidecar.Container{Image: "caddy:2"},
		},
		"reserved name": {
			Description:   "Runtime service",
			Architectures: []string{"amd64"},
			Container:     sidecar.Container{Name: "database", Image: "caddy:2"},
		},
		"unsupported field value": {
			Description:   "Runtime service",
			Architectures: []string{"amd64"},
			Container: sidecar.Container{
				Image: "caddy:2", ImagePullPolicy: "Sometimes",
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// When
			_, err := sidecar.NewCatalog(map[string]sidecar.Entry{"example": entry})
			// Then
			require.ErrorContains(t, err, "sidecars.example")
		})
	}
}

func TestRuntimeEntriesIsolateTheirSource(t *testing.T) {
	t.Parallel()
	// Given
	entries := map[string]sidecar.Entry{"example": {
		Description:   "Runtime service",
		Architectures: []string{"amd64"},
		Container:     sidecar.Container{Image: "caddy:2"},
	}}
	catalog, err := sidecar.NewCatalog(entries)
	require.NoError(t, err)
	// When
	edited := entries["example"]
	edited.Container.Image = "edited"
	entries["example"] = edited
	entries["other"] = edited
	selected, err := catalog.Resolve("example", "amd64")
	// Then
	require.NoError(t, err)
	require.Equal(t, "caddy:2", selected.Image)
	require.NotContains(t, catalog.Entries(), "other")
}
