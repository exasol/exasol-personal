// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMCPCommandBindsSidecarSubcommandsToTheMCPServer(t *testing.T) {
	t.Parallel()
	// Given
	command := newMCPCommand(&CommonFlags{})
	// When / Then
	for _, name := range []string{"enable", "status", "disable"} {
		child, _, err := command.Find([]string{name})
		require.NoError(t, err)
		require.Equal(t, name, child.Name())
		for _, flag := range []string{"deployment", "deployment-dir", "json"} {
			require.NotNil(t, child.Flags().Lookup(flag), "%s missing --%s", name, flag)
		}
		require.NoError(t, child.Args(child, nil))
		require.Error(t, child.Args(child, []string{"mcp"}))
		require.True(t, deploymentFileLoggingIsRequired(child))
	}
	enable, _, _ := command.Find([]string{"enable"})
	require.NotNil(t, enable.Flags().Lookup("no-wait"))
	disable, _, _ := command.Find([]string{"disable"})
	require.NotNil(t, disable.Flags().Lookup("remove"))
}
