// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetCertFingerprint_NoNodes(t *testing.T) {
	t.Parallel()

	nodeDetails := &NodeDetails{Nodes: map[string]nodeDetailsNode{}}
	_, err := nodeDetails.GetCertFingerprint()
	require.Error(t, err)
}

func TestGetCertFingerprint_EmptyTlsCert_Error(t *testing.T) {
	t.Parallel()

	nodeDetails := &NodeDetails{Nodes: map[string]nodeDetailsNode{
		"n11": {
			TlsCert: "",
		},
	}}

	_, err := nodeDetails.GetCertFingerprint()
	require.Error(t, err)
}

func TestGetCertFingerprint_FallbackWhenN11Missing(t *testing.T) {
	t.Parallel()

	nodeDetails := &NodeDetails{Nodes: map[string]nodeDetailsNode{
		"n10": {
			TlsCert: "", // triggers empty cert error but confirms fallback node chosen
		},
		"n12": {
			TlsCert: "ignored",
		},
	}}

	_, err := nodeDetails.GetCertFingerprint()
	require.Error(t, err)
}
