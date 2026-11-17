// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package sidecar_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/sidecar"
	"github.com/stretchr/testify/require"
)

const volumeDocument = `version: 1
sidecars:
  example:
    container:
      image: example:1
      env:
        - name: PASSWORD_FILE
          value: /run/exasol-database/password
      volumeMounts:
        - name: database
          mountPath: /run/exasol-database
          readOnly: true
    volumes:
      - name: database
        secret:
          secretName: exasol-database
          items:
            - key: password
              path: password
`

func TestSavedDocumentCarriesSecretVolumes(t *testing.T) {
	t.Parallel()
	// When
	document, err := sidecar.Parse([]byte(volumeDocument))
	// Then
	require.NoError(t, err)
	definition := document.Sidecars["example"]
	require.Equal(t, []sidecar.VolumeMount{
		{Name: "database", MountPath: "/run/exasol-database", ReadOnly: true},
	}, definition.Container.VolumeMounts)
	require.Equal(t, []sidecar.Volume{{Name: "database", Secret: &sidecar.SecretVolume{
		SecretName: sidecar.DatabaseSecret,
		Items:      []sidecar.KeyToPath{{Key: "password", Path: "password"}},
	}}}, definition.Volumes)
}

func TestSecretVolumeNamesTheOffendingField(t *testing.T) {
	t.Parallel()
	for _, fixture := range []struct{ name, old, replacement, message string }{
		{
			"unknown volume", "        - name: database\n          mountPath",
			"        - name: other\n          mountPath",
			"sidecars.example.container.volumeMounts[0].name: names no volume",
		},
		{
			"relative mount", "mountPath: /run/exasol-database", "mountPath: run/database",
			"sidecars.example.container.volumeMounts[0].mountPath: expected a unique absolute path",
		},
		{
			"root mount", "mountPath: /run/exasol-database", "mountPath: /",
			"sidecars.example.container.volumeMounts[0].mountPath",
		},
		{
			"missing source", "        secret:\n          secretName: exasol-database\n",
			"        secret:\n          secretName: ''\n",
			"sidecars.example.volumes[0].secret.secretName: a secret source is required",
		},
		{
			"escaping path", "              path: password", "              path: ../password",
			"sidecars.example.volumes[0].secret.items[0].path",
		},
		{
			"absolute path", "              path: password", "              path: /password",
			"sidecars.example.volumes[0].secret.items[0].path",
		},
		{
			"missing key", "            - key: password", "            - key: ''",
			"sidecars.example.volumes[0].secret.items[0].key: is required",
		},
		{
			"invalid volume name", "      - name: database\n        secret",
			"      - name: Data_Base\n        secret",
			"sidecars.example.volumes[0].name: expected a unique DNS label",
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			// Given
			require.Contains(t, volumeDocument, fixture.old)
			data := strings.Replace(volumeDocument, fixture.old, fixture.replacement, 1)
			// When
			_, err := sidecar.Parse([]byte(data))
			// Then
			require.ErrorContains(t, err, fixture.message)
		})
	}
}

func TestResolveFilesPlacesItemsBelowTheirMount(t *testing.T) {
	t.Parallel()
	// Given
	document, err := sidecar.Parse([]byte(volumeDocument))
	require.NoError(t, err)
	// When
	files, err := sidecar.ResolveFiles(document.Sidecars["example"], sidecar.Secrets{
		sidecar.DatabaseSecret: {"password": {Value: "secret", Sensitive: true}},
	})
	// Then
	require.NoError(t, err)
	require.Equal(t, []sidecar.File{{
		MountPath: "/run/exasol-database", Path: "/run/exasol-database/password",
		Content: "secret", Sensitive: true,
	}}, files)
}

func TestResolvedDiagnosticsWithholdSensitiveFiles(t *testing.T) {
	t.Parallel()
	// Given
	resolved, err := sidecar.Resolve(sidecar.Container{Name: "example", Image: "example:1"}, nil)
	require.NoError(t, err)
	// When
	message := resolved.WithFiles([]sidecar.File{
		{Content: "disposable-secret", Sensitive: true}, {Content: "database"},
	}).Redact("database refused disposable-secret")
	// Then
	require.Equal(t, "database refused [redacted]", message)
}

func TestResolveFilesRequiresTheSecretSource(t *testing.T) {
	t.Parallel()
	for _, optional := range []bool{false, true} {
		t.Run(fmt.Sprint("optional=", optional), func(t *testing.T) {
			t.Parallel()
			// Given
			document, err := sidecar.Parse([]byte(volumeDocument))
			require.NoError(t, err)
			definition := document.Sidecars["example"]
			definition.Volumes[0].Secret.Items = nil
			definition.Volumes[0].Secret.SecretName = "unknown"
			definition.Volumes[0].Secret.Optional = optional
			// When
			files, err := sidecar.ResolveFiles(definition, sidecar.Secrets{
				sidecar.DatabaseSecret: {"password": {Value: "secret", Sensitive: true}},
			})
			// Then
			if optional {
				require.NoError(t, err)
				require.Empty(t, files)
			} else {
				require.ErrorContains(t, err, "required secret unknown is unavailable")
			}
		})
	}
}
