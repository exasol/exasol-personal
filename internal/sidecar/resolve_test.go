// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package sidecar_test

import (
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/sidecar"
	"github.com/stretchr/testify/require"
)

func referenceContainer(optional bool) sidecar.Container {
	return sidecar.Container{Name: "example", Image: "caddy:2", Env: []sidecar.EnvVar{{
		Name: "PASSWORD", ValueFrom: &sidecar.EnvSource{SecretKeyRef: &sidecar.SecretKeyRef{
			Name: sidecar.DatabaseSource, Key: "password", Optional: optional,
		}},
	}}}
}

func TestSidecarRequiredReferenceError(t *testing.T) {
	t.Parallel()
	// Given
	container := referenceContainer(false)
	// When
	_, err := sidecar.Resolve(container, nil)
	// Then
	require.Error(t, err, "expected required reference failure")
	for _, field := range []string{"example", "exasol-database", "password"} {
		require.Contains(t, err.Error(), field)
	}
}

func TestSidecarKubernetesExpansion(t *testing.T) {
	t.Parallel()
	for _, fixture := range []struct{ input, want string }{
		{"$(FIRST)-$(SECOND)", "one-one/$(LATER)"},
		{"$$(FIRST)", "$(FIRST)"},
		{"$$$(FIRST)", "$one"},
		{"$(UNKNOWN)", "$(UNKNOWN)"},
		{"$(FIRST", "$(FIRST"},
		{"tail$", "tail$"},
	} {
		t.Run(fixture.input, func(t *testing.T) {
			t.Parallel()
			// Given
			first, second, later := "one", "$(FIRST)/$(LATER)", "three"
			container := sidecar.Container{
				Name: "example", Image: "caddy:2",
				Env: []sidecar.EnvVar{
					{Name: "FIRST", Value: &first},
					{Name: "SECOND", Value: &second},
					{Name: "LATER", Value: &later},
				},
				Command: []string{fixture.input}, Args: []string{fixture.input},
			}
			// When
			result, err := sidecar.Resolve(container, nil)
			// Then
			require.NoError(t, err)
			if result.Container.Command[0] != fixture.want ||
				result.Container.Args[0] != fixture.want ||
				*result.Container.Env[1].Value != "one/$(LATER)" {
				t.Fatalf("unexpected expansion: %+v", result.Container)
			}
		})
	}
}

func TestResolvedDiagnosticsRedactAllValueForms(t *testing.T) {
	t.Parallel()
	// Given
	container := referenceContainer(false)
	secret := "test'password\nsecond"
	result, err := sidecar.Resolve(container, sidecar.Sources{
		sidecar.DatabaseSource: {"password": secret},
	})
	require.NoError(t, err)
	// When
	message := result.Redact(secret + " test'password\\nsecond test'\"'\"'password\nsecond")
	// Then
	if strings.Contains(message, "password") || strings.Contains(message, "second") {
		t.Fatalf("credential leaked: %q", message)
	}
	if container.Env[0].ValueFrom == nil {
		t.Fatal("saved reference mutated")
	}
}
