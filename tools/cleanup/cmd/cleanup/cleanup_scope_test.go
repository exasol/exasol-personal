// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRenderCleanupScopeIncludesStatuses(t *testing.T) {
	t.Parallel()

	scope := cleanupScope{
		Providers: []cleanupScopeProvider{
			{
				Provider: "aws",
				Location: "us-east-1",
				Owner:    "(caller)",
				Status:   providerStatusSearched,
				Reason:   "",
			},
			{
				Provider: "aws",
				Location: "eu-central-1",
				Owner:    "(caller)",
				Status:   providerStatusSearched,
				Reason:   "",
			},
			{
				Provider: "exoscale",
				Location: "ch-gva-2",
				Owner:    "*",
				Status:   providerStatusSkipped,
				Reason:   providerReasonNotSelected,
			},
		},
	}

	var output bytes.Buffer

	// Given a built scope table
	// When it is rendered for human-readable output
	renderCleanupScope(&output, scope)

	// Then it lists providers, status, and reason
	rendered := strings.ToLower(output.String())
	for _, expected := range []string{"scope:", "aws", "us-east-1", "eu-central-1", "exoscale", "searched", "skipped", "not selected"} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("rendered scope missing %q: %s", expected, output.String())
		}
	}
}
