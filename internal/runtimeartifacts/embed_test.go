// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package runtimeartifacts

import "testing"

//nolint:paralleltest // embeddedResources is shared; concurrent Register calls aren't safe.
func TestRegister_StoresNonEmptyData(t *testing.T) {
	// Given
	const resourceID = "embed-test-store"
	t.Cleanup(func() {
		delete(embeddedResources, resourceID)
		delete(embeddedHashes, resourceID)
	})

	// When
	Register(resourceID, []byte("data"), "deadbeef")

	// Then
	data, dataRegistered := lookupEmbedded(resourceID)
	if !dataRegistered || string(data) != "data" {
		t.Fatalf("expected registered data to be retrievable, got %q, ok=%v", data, dataRegistered)
	}
	hash, hashRegistered := lookupEmbeddedHash(resourceID)
	if !hashRegistered || hash != "deadbeef" {
		t.Fatalf("expected registered hash to be retrievable, got %q, ok=%v", hash, hashRegistered)
	}
}

//nolint:paralleltest // embeddedResources is shared; concurrent Register calls aren't safe.
func TestRegister_EmptyDataIsNoOp(t *testing.T) {
	// Given
	const resourceID = "embed-test-noop"
	t.Cleanup(func() {
		delete(embeddedResources, resourceID)
		delete(embeddedHashes, resourceID)
	})

	// When
	Register(resourceID, nil, "deadbeef")
	Register(resourceID, []byte{}, "deadbeef")

	// Then
	if _, ok := lookupEmbedded(resourceID); ok {
		t.Fatal("expected empty data to never register a resource")
	}
	if _, ok := lookupEmbeddedHash(resourceID); ok {
		t.Fatal("expected empty data to never register a hash either")
	}
}
