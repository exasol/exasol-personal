// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"os"
	"runtime"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/sidecar"
	"github.com/stretchr/testify/require"
)

func sidecarTestCatalog(t *testing.T) *sidecar.Catalog {
	t.Helper()
	catalog, err := sidecar.LoadCatalog([]byte(`version: 1
sidecars:
  example:
    description: Test service
    architectures: [amd64, arm64]
    container:
      image: docker.io/library/caddy:2-alpine
`))
	require.NoError(t, err)

	return catalog
}

func materializeTestSidecar(
	t *testing.T,
	deployment config.DeploymentDir,
	catalog *sidecar.Catalog,
) {
	t.Helper()
	err := withDeploymentExclusiveLock(
		context.Background(),
		deployment,
		func(directory config.DeploymentDir) error {
			document, err := config.ReadSidecars(directory)
			if err != nil {
				return err
			}
			_, err = materializeSidecarLocked(directory, document, catalog, "example", "amd64")
			return err
		},
	)
	require.NoError(t, err)
}

func TestSidecarEnableMaterializesTemplate(t *testing.T) {
	t.Parallel()
	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	catalog := sidecarTestCatalog(t)
	// When
	materializeTestSidecar(t, deployment, catalog)
	// Then
	document, err := config.ReadSidecars(deployment)
	require.NoError(t, err)
	if len(document.Containers) != 1 || document.Containers[0].Name != "example" ||
		document.Containers[0].Image != "docker.io/library/caddy:2-alpine" ||
		document.Containers[0].RestartPolicy != sidecar.OnFailure {
		t.Fatalf("unexpected definition: %+v", document)
	}
	info, err := os.Stat(deployment.SidecarsPath())
	require.NoError(t, err)
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("file permissions = %o", info.Mode().Perm())
	}
}

func TestSidecarEnablePreservesEdits(t *testing.T) {
	t.Parallel()
	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	catalog := sidecarTestCatalog(t)
	materializeTestSidecar(t, deployment, catalog)
	edited := []byte(
		"version: 1\ncontainers:\n- name: example\n  image: caddy:edited\n  args: [custom]\n",
	)
	require.NoError(t, os.WriteFile(deployment.SidecarsPath(), edited, 0o600))
	// When
	materializeTestSidecar(t, deployment, catalog)
	// Then
	actual, err := os.ReadFile(deployment.SidecarsPath())
	require.NoError(t, err)
	if string(actual) != string(edited) {
		t.Fatalf("definition changed: %s", actual)
	}
}
