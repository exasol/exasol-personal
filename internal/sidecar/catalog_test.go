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
	*selected.Container.Env[0].Value = "edited"
	again, err := catalog.Resolve("example", "amd64")
	require.NoError(t, err)
	// Then
	if again.Container.Name != "example" || again.Container.RestartPolicy != "OnFailure" ||
		again.Container.ImagePullPolicy != "IfNotPresent" ||
		*again.Container.Env[0].Value != "original" {
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
		{
			"enabled", "    container:", "    enabled: true\n    container:",
			"sidecars.example.enabled",
		},
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

// containerDocument saves one sidecar under the key "example", so a fixture
// supplies only the container body it needs.
func containerDocument(container string) []byte {
	return []byte("version: 1\nsidecars:\n  example:\n    container:\n      " + container + "\n")
}

func TestSidecarInvalidValuesNameOffendingField(t *testing.T) {
	t.Parallel()
	for _, fixture := range []struct{ name, container, location string }{
		{"null", "image: null", "sidecars.example.container.image"},
		{"image", "image: '-invalid'", "sidecars.example.container.image"},
		{"pull", "image: caddy\n      imagePullPolicy: Sometimes", "imagePullPolicy"},
		{"restart", "image: caddy\n      restartPolicy: Sometimes", "restartPolicy"},
		{"env_duplicate", "image: caddy\n      env: [{name: X}, {name: X}]", "env[1].name"},
		{"env_conflict", "image: caddy\n      env: [{name: X, value: '', " +
			"valueFrom: {secretKeyRef: {name: x, key: y}}}]", "env[0]"},
		{
			"env_source", "image: caddy\n      env: [{name: X, valueFrom: {}}]",
			"env[0].valueFrom.secretKeyRef",
		},
		{"port", "image: caddy\n      ports: [{containerPort: 0}]", "ports[0].containerPort"},
		{
			"host_port", "image: caddy\n      ports: [{containerPort: 80, hostPort: 65536}]",
			"ports[0].hostPort",
		},
		{
			"protocol", "image: caddy\n      ports: [{containerPort: 80, protocol: UDP}]",
			"ports[0].protocol",
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			// Given
			data := containerDocument(fixture.container)
			// When
			_, err := sidecar.Parse(data)
			// Then
			if err == nil || !strings.Contains(err.Error(), fixture.location) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestSavedEntryKeyNamesItsContainer(t *testing.T) {
	t.Parallel()
	// Given
	data := containerDocument("image: caddy:2")
	// When
	document, err := sidecar.Parse(data)
	// Then
	require.NoError(t, err)
	require.Equal(t, "example", document.Sidecars["example"].Container.Name)
}

func TestSavedContainerNameContradictingItsKeyFailsValidation(t *testing.T) {
	t.Parallel()
	// Given
	data := containerDocument("name: another\n      image: caddy:2")
	// When
	_, err := sidecar.Parse(data)
	// Then
	require.ErrorContains(t, err, "sidecars.example.container.name")
}

func TestSidecarUnsupportedConfigurationIsRejected(t *testing.T) {
	t.Parallel()
	for _, fixture := range []struct{ name, container string }{
		{"unknown field", "image: caddy\n      resources: {}"},
		{"unknown nested field", "image: caddy\n      env: [{name: X, valueFrom: {fieldRef: {}}}]"},
		{"duplicate field", "image: caddy\n      image: caddy"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			// Given
			data := containerDocument(fixture.container)
			// When
			_, err := sidecar.Parse(data)
			// Then
			require.Error(t, err)
		})
	}
}

func TestDocumentBoundaries(t *testing.T) {
	t.Parallel()
	for _, data := range []string{
		"", "[", "version: 1", "version: 1\nsidecars: null",
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

func TestHostIPDefaultsToItsCanonicalForm(t *testing.T) {
	t.Parallel()
	for _, fixture := range []struct{ hostIP, want string }{
		{"0:0:0:0:0:0:0:1", "::1"},
		{"::0001", "::1"},
		{"0000:0000:0000:0000:0000:0000:0000:0001", "::1"},
		{"127.0.0.1", "127.0.0.1"},
		{"loopback", "loopback"},
	} {
		t.Run(fixture.hostIP, func(t *testing.T) {
			t.Parallel()
			// Given
			container := sidecar.Container{Image: "caddy:2", Ports: []sidecar.Port{
				{ContainerPort: 8080, HostPort: 18080, HostIP: fixture.hostIP},
			}}
			// When
			actual := container.Defaults()
			// Then
			require.Equal(t, fixture.want, actual.Ports[0].HostIP)
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
	require.Equal(t, "docker.io/library/redis:8", selected.Container.Image)
	require.Equal(t, "example", selected.Container.Name)
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
	require.Equal(t, "docker.io/library/caddy:2-alpine", selected.Container.Image)
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
	require.Equal(t, "caddy:2", selected.Container.Image)
	require.NotContains(t, catalog.Entries(), "other")
}

func TestCatalogDistinguishesUnknownNameFromArchitecture(t *testing.T) {
	t.Parallel()
	// Given
	catalog, err := sidecar.LoadCatalog([]byte(catalogYAML))
	require.NoError(t, err)
	// When
	_, unknown := catalog.Resolve("absent", "amd64")
	_, mismatched := catalog.Resolve("example", "s390x")
	// Then
	if unknown == nil || !strings.Contains(unknown.Error(), "not in the catalog") {
		t.Fatalf("unknown name: %v", unknown)
	}
	if mismatched == nil || !strings.Contains(mismatched.Error(), "not available for s390x") {
		t.Fatalf("architecture mismatch: %v", mismatched)
	}
}

const endpointCatalogYAML = `version: 1
sidecars:
  example:
    description: Example service
    architectures: [amd64, arm64]
    container:
      image: docker.io/library/caddy:2-alpine
      ports:
        - name: api
          containerPort: 8080
          hostPort: 18080
    endpoints:
      - port: api
        scheme: http
        path: /v1
`

func endpointDocument(endpoints string) []byte {
	return []byte("version: 1\nsidecars:\n  example:\n    container:\n      image: caddy\n" +
		"      ports:\n        - name: api\n          containerPort: 8080\n" +
		"          hostPort: 18080\n" + endpoints)
}

func TestCatalogResolvesEndpointDeclarations(t *testing.T) {
	t.Parallel()
	// Given
	catalog, err := sidecar.LoadCatalog([]byte(endpointCatalogYAML))
	require.NoError(t, err)
	// When
	selected, err := catalog.Resolve("example", "amd64")
	// Then
	require.NoError(t, err)
	require.Equal(
		t,
		[]sidecar.Endpoint{{Port: "api", Scheme: "http", Path: "/v1"}},
		selected.Endpoints,
	)
}

func TestSavedDocumentCarriesEndpointDeclarations(t *testing.T) {
	t.Parallel()
	// Given
	data := endpointDocument("    endpoints:\n      - port: api\n        scheme: http\n")
	// When
	document, err := sidecar.Parse(data)
	// Then
	require.NoError(t, err)
	require.Equal(
		t,
		[]sidecar.Endpoint{{Port: "api", Scheme: "http"}},
		document.Sidecars["example"].Endpoints,
	)
}

func TestSavedDocumentCarriesEnabledState(t *testing.T) {
	t.Parallel()
	for _, fixture := range []struct {
		name, field string
		enabled     bool
	}{
		{"disabled", "    enabled: false\n", false},
		{"enabled", "    enabled: true\n", true},
		{"field-less", "", true},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			// Given
			data := []byte("version: 1\nsidecars:\n  example:\n" + fixture.field +
				"    container:\n      image: caddy:2\n")
			// When
			document, err := sidecar.Parse(data)
			// Then
			require.NoError(t, err)
			require.Equal(t, fixture.enabled, document.Sidecars["example"].IsEnabled())
			require.Len(t, document.Containers(), map[bool]int{true: 1, false: 0}[fixture.enabled])
		})
	}
}

func TestSavedDocumentWithoutEndpointDeclarations(t *testing.T) {
	t.Parallel()
	// Given
	data := endpointDocument("")
	// When
	document, err := sidecar.Parse(data)
	// Then
	require.NoError(t, err)
	require.Nil(t, document.Sidecars["example"].Endpoints)
}

func TestEndpointDeclarationNamesTheOffendingField(t *testing.T) {
	t.Parallel()
	const declared = "    endpoints:\n      - port: "
	const unknownPort = ".port: expected the name of a container port"
	for _, fixture := range []struct{ name, endpoints, message string }{
		{
			"unknown port", declared + "other\n        scheme: http\n",
			"sidecars.example.endpoints[0]" + unknownPort,
		},
		{
			"unnamed port", declared + "''\n        scheme: http\n",
			"sidecars.example.endpoints[0]" + unknownPort,
		},
		{
			"duplicate port", declared + "api\n        scheme: http\n" +
				"      - port: api\n        scheme: https\n",
			"sidecars.example.endpoints[1].port: expected a unique container port name",
		},
		{
			"missing scheme", declared + "api\n",
			"sidecars.example.endpoints[0].scheme: expected an application protocol such as http",
		},
		{
			"malformed scheme", declared + "api\n        scheme: 'ht tp'\n",
			"sidecars.example.endpoints[0].scheme: expected an application protocol such as http",
		},
		{
			"relative path", declared + "api\n        scheme: http\n        path: v1\n",
			"sidecars.example.endpoints[0].path: expected a path beginning with /",
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			// Given
			data := endpointDocument(fixture.endpoints)
			// When
			_, err := sidecar.Parse(data)
			// Then
			if err == nil || !strings.Contains(err.Error(), fixture.message) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestCatalogEndpointDeclarationFollowsDocumentValidation(t *testing.T) {
	t.Parallel()
	// Given
	invalid := strings.Replace(endpointCatalogYAML, "port: api", "port: other", 1)
	// When
	_, err := sidecar.LoadCatalog([]byte(invalid))
	// Then
	require.ErrorContains(t, err, "sidecars.example.endpoints[0].port")
}
