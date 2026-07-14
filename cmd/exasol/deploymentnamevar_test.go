// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import "testing"

func TestDeploymentNameValue_AcceptsSafeCharacters(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"default", "staging", "prod-aws", "test_1", "ABC123"} {
		var target string

		value := NewDeploymentNameValue(&target)
		if err := value.Set(name); err != nil {
			t.Fatalf("expected %q to be accepted, got error: %v", name, err)
		}
		if target != name {
			t.Fatalf("expected target %q, got %q", name, target)
		}
	}
}

func TestDeploymentNameValue_RejectsUnsafeCharacters(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"", "bad/name", "../escape", "bad name", "bad.name", "..", "/"} {
		var target string

		value := NewDeploymentNameValue(&target)
		if err := value.Set(name); err == nil {
			t.Fatalf("expected %q to be rejected", name)
		}
	}
}
