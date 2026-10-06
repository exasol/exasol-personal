//go:build sidecar_test_catalog

// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package sidecars_test

import (
	"testing"

	"github.com/exasol/exasol-personal/internal/sidecar"
	"github.com/exasol/exasol-personal/tests/fixtures/sidecars"
	"github.com/stretchr/testify/require"
)

func TestFixtureCatalog(t *testing.T) {
	t.Parallel()
	// Given
	catalog, err := sidecar.LoadCatalog(sidecars.Catalog)
	require.NoError(t, err)
	for _, architecture := range []string{"amd64", "arm64"} {
		t.Run(architecture, func(t *testing.T) {
			t.Parallel()
			// When
			container, err := catalog.Resolve("caddy", architecture)
			// Then
			require.NoError(t, err)
			require.Equal(t, "caddy", container.Name)
			require.Len(t, container.Env, 5)
			require.Equal(t, "OnFailure", container.RestartPolicy)
			require.Equal(t, 18080, container.Ports[0].HostPort)
		})
	}
}
