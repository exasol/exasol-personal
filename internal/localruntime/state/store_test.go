// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package state

import (
	"path/filepath"
	"testing"
)

func TestStoreReadMissing_ReturnsEmptyState(t *testing.T) {
	t.Parallel()

	// Given
	store := NewStore(filepath.Join(t.TempDir(), "state.json"))

	// When
	state, err := store.Read()
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if state == nil {
		t.Fatal("expected non-nil state")
	}
	if len(state.Ports) != 0 {
		t.Fatalf("expected empty ports map, got %#v", state.Ports)
	}
}

func TestStoreWriteRoundTrip(t *testing.T) {
	t.Parallel()

	// Given
	store := NewStore(filepath.Join(t.TempDir(), "state.json"))
	expected := &State{
		Ports: map[string]int{"db": 8563},
		Payload: &PayloadRef{
			Version:      "1.2.3",
			Architecture: "arm64",
			Checksum:     "abc",
			CachePath:    "/tmp/payload",
			Boot: &PayloadBootRef{
				KernelPath: "/tmp/kernel",
				InitrdPath: "/tmp/initrd",
			},
		},
	}

	// When
	if err := store.Write(expected); err != nil {
		t.Fatalf("expected write to succeed, got %v", err)
	}
	actual, err := store.Read()
	// Then
	if err != nil {
		t.Fatalf("expected read to succeed, got %v", err)
	}
	if actual.Ports["db"] != expected.Ports["db"] {
		t.Fatalf("unexpected db port: got %d expected %d", actual.Ports["db"], expected.Ports["db"])
	}
	if actual.Payload == nil {
		t.Fatal("expected payload to be present")
	}
	if actual.Payload.Version != expected.Payload.Version {
		t.Fatalf(
			"unexpected payload version: got %q expected %q",
			actual.Payload.Version,
			expected.Payload.Version,
		)
	}
	if actual.Payload.Architecture != expected.Payload.Architecture {
		t.Fatalf(
			"unexpected payload architecture: got %q expected %q",
			actual.Payload.Architecture,
			expected.Payload.Architecture,
		)
	}
	if actual.Payload.Boot == nil {
		t.Fatal("expected payload boot assets to be present")
	}
	if actual.Payload.Boot.KernelPath != expected.Payload.Boot.KernelPath {
		t.Fatalf(
			"unexpected payload kernel path: got %q expected %q",
			actual.Payload.Boot.KernelPath,
			expected.Payload.Boot.KernelPath,
		)
	}
	if actual.Payload.Boot.InitrdPath != expected.Payload.Boot.InitrdPath {
		t.Fatalf(
			"unexpected payload initrd path: got %q expected %q",
			actual.Payload.Boot.InitrdPath,
			expected.Payload.Boot.InitrdPath,
		)
	}
}
