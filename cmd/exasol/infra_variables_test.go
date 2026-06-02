// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"testing"

	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/spf13/cobra"
)

//nolint:paralleltest // This test temporarily replaces package-global infraFlagToVarName.
func TestRegisterInfrastructureVariableFlagsRegistersAllSupportedTypes(t *testing.T) {
	// Given
	originalFlagMap := infraFlagToVarName
	infraFlagToVarName = map[string]string{}
	t.Cleanup(func() {
		infraFlagToVarName = originalFlagMap
	})

	cmd := &cobra.Command{Use: "test"}
	vars := map[string]deploy.ConfigVariableDefinition{
		"cluster_size": {
			Name: "cluster_size",
			Type: deploy.ConfigVariableTypeNumber,
		},
		"instance_type": {
			Name: "instance_type",
			Type: deploy.ConfigVariableTypeString,
		},
		"use_private_subnet": {
			Name: "use_private_subnet",
			Type: deploy.ConfigVariableTypeBool,
		},
	}

	// When
	if err := registerInfrastructureVariableFlags(
		[]*cobra.Command{cmd},
		vars,
		"test-preset",
	); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Then
	for _, expected := range []string{
		"cluster-size",
		"instance-type",
		"use-private-subnet",
	} {
		if cmd.Flags().Lookup(expected) == nil {
			t.Fatalf("expected flag --%s to be registered", expected)
		}
	}
}
