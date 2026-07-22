// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package runtimeartifacts

import "testing"

//nolint:paralleltest // embeddedResources is shared; concurrent Register calls aren't safe.
func TestRegister_StoresNonEmptyData(t *testing.T) {
	// Given
	const resourceID = "embed-test-store"
	t.Cleanup(func() { delete(embeddedResources, resourceID) })

	// When
	Register(resourceID, []byte("data"))

	// Then
	data, ok := lookupEmbedded(resourceID)
	if !ok || string(data) != "data" {
		t.Fatalf("expected registered data to be retrievable, got %q, ok=%v", data, ok)
	}
}

//nolint:paralleltest // embeddedResources is shared; concurrent Register calls aren't safe.
func TestRegister_EmptyDataIsNoOp(t *testing.T) {
	// Given
	const resourceID = "embed-test-noop"
	t.Cleanup(func() { delete(embeddedResources, resourceID) })

	// When
	Register(resourceID, nil)
	Register(resourceID, []byte{})

	// Then
	if _, ok := lookupEmbedded(resourceID); ok {
		t.Fatal("expected empty data to never register a resource")
	}
}
