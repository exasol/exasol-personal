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

// sidecarTestCredentialCatalog describes a service that consumes the
// database password, so tests can observe resolved values reaching a
// container.
func sidecarTestCredentialCatalog(t *testing.T) *sidecar.Catalog {
	t.Helper()
	catalog, err := sidecar.LoadCatalog([]byte(`version: 1
sidecars:
  example:
    description: Test service
    architectures: [amd64, arm64]
    container:
      image: docker.io/library/caddy:2-alpine
      env:
        - name: DB_PASSWORD
          valueFrom:
            secretKeyRef:
              name: exasol-database
              key: password
`))
	require.NoError(t, err)

	return catalog
}

// sidecarTestPublishedCatalog describes a service that publishes a port, so
// tests can observe the endpoint a user would reach.
func sidecarTestPublishedCatalog(t *testing.T) *sidecar.Catalog {
	t.Helper()
	catalog, err := sidecar.LoadCatalog([]byte(`version: 1
sidecars:
  example:
    description: Test service
    architectures: [amd64, arm64]
    container:
      image: docker.io/library/caddy:2-alpine
      ports:
        - containerPort: 8080
          hostPort: 18080
          hostIP: 127.0.0.1
`))
	require.NoError(t, err)

	return catalog
}

func materializeTestSidecar(
	t *testing.T,
	deployment config.DeploymentDir,
	catalog sidecar.Resolver,
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
			if containsSidecar(document, "example") {
				return nil
			}
			container, err := catalog.Resolve("example", runtime.GOARCH)
			if err != nil {
				return err
			}
			document.Containers = append(document.Containers, container)

			return config.WriteSidecars(directory, document)
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

func TestSidecarEnableStoresConnectionReferences(t *testing.T) {
	t.Parallel()
	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	catalog := sidecarTestCatalog(t)
	entry := catalog.Sidecars["example"]
	entry.Container.Env = []sidecar.EnvVar{{
		Name: "DB_PASSWORD",
		ValueFrom: &sidecar.EnvSource{SecretKeyRef: &sidecar.SecretKeyRef{
			Name: sidecar.DatabaseSource, Key: "password",
		}},
	}}
	catalog.Sidecars["example"] = entry
	// When
	materializeTestSidecar(t, deployment, catalog)
	// Then
	document, err := config.ReadSidecars(deployment)
	require.NoError(t, err)
	require.Len(t, document.Containers, 1)
	saved := document.Containers[0].Env
	require.Len(t, saved, 1)
	require.Nil(t, saved[0].Value)
	require.Equal(t, sidecar.DatabaseSource, saved[0].ValueFrom.SecretKeyRef.Name)
	require.Equal(t, "password", saved[0].ValueFrom.SecretKeyRef.Key)
}
