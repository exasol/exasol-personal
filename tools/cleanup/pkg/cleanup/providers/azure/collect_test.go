// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package azure

import (
	"testing"
	"time"
)

func ptr(s string) *string { return &s }

func tags(m map[string]string) map[string]*string {
	out := make(map[string]*string, len(m))
	for k, v := range m {
		out[k] = ptr(v)
	}

	return out
}

func TestClassifyAzureType(t *testing.T) {
	t.Parallel()

	cases := map[string]ResourceType{
		"Microsoft.Compute/virtualMachines":       ResourceVirtualMachine,
		"Microsoft.Compute/disks":                 ResourceDisk,
		"Microsoft.Network/networkInterfaces":     ResourceNetworkIface,
		"Microsoft.Network/publicIPAddresses":     ResourcePublicIP,
		"Microsoft.Network/virtualNetworks":       ResourceVirtualNetwork,
		"Microsoft.Network/networkSecurityGroups": ResourceSecurityGroup,
		"Microsoft.Storage/storageAccounts":       ResourceStorageAccount,
		"Microsoft.SomethingElse/widgets":         ResourceGeneric,
	}
	for azureType, want := range cases {
		if got := classifyAzureType(azureType); got != want {
			t.Fatalf("classifyAzureType(%q) = %q, want %q", azureType, got, want)
		}
	}
}

func TestMatchesDeploymentTags(t *testing.T) {
	t.Parallel()

	modern := tags(map[string]string{tagProject: projectValue, tagDeployment: "exasol-12345678"})
	if !matchesDeploymentTags(modern, false) {
		t.Fatal("modern deployment should match in default mode")
	}

	missingProject := tags(map[string]string{tagDeployment: "exasol-12345678"})
	if matchesDeploymentTags(missingProject, false) {
		t.Fatal("deployment without Project tag must not match in default mode")
	}
	if !matchesDeploymentTags(missingProject, true) {
		t.Fatal("deployment without Project tag should match in legacy mode")
	}

	badID := tags(map[string]string{tagProject: projectValue, tagDeployment: "exasol-XYZ"})
	if matchesDeploymentTags(badID, false) {
		t.Fatal("malformed deployment id must not match")
	}

	unrelated := tags(map[string]string{tagProject: "something-else", tagDeployment: "exasol-12345678"})
	if matchesDeploymentTags(unrelated, false) {
		t.Fatal("unrelated Project value must not match in default mode")
	}
}

func TestOwnerMatchesFilter(t *testing.T) {
	t.Parallel()

	if !ownerMatchesFilter("anyone", "") || !ownerMatchesFilter("anyone", "*") {
		t.Fatal("empty and * filters must match any owner")
	}
	if !ownerMatchesFilter("alice@example.com", "alice*") {
		t.Fatal("wildcard prefix should match")
	}
	if ownerMatchesFilter("bob@example.com", "alice*") {
		t.Fatal("non-matching owner should be rejected")
	}
}

func TestMatchesLocation(t *testing.T) {
	t.Parallel()

	if !matchesLocation(DefaultLocation, "westeurope") || !matchesLocation("", "westeurope") {
		t.Fatal("the all/empty sentinel must match any location")
	}
	if !matchesLocation("WestEurope", "westeurope") {
		t.Fatal("location match should be case-insensitive")
	}
	if matchesLocation("northeurope", "westeurope") {
		t.Fatal("different locations must not match")
	}
}

func TestParseCreatedAt(t *testing.T) {
	t.Parallel()

	if !parseCreatedAt("").IsZero() {
		t.Fatal("empty value should parse to zero time")
	}
	if !parseCreatedAt("not-a-date").IsZero() {
		t.Fatal("malformed value should parse to zero time")
	}
	got := parseCreatedAt("2026-07-08T10:00:00Z")
	if !got.Equal(time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("parseCreatedAt = %v, want 2026-07-08T10:00:00Z", got)
	}
}

func TestTagHelpers(t *testing.T) {
	t.Parallel()

	in := tags(map[string]string{tagOwner: "alice", tagDeployment: "exasol-12345678"})
	if got := tagValue(in, tagOwner); got != "alice" {
		t.Fatalf("tagValue(Owner) = %q, want alice", got)
	}
	if got := tagValue(in, "absent"); got != "" {
		t.Fatalf("tagValue(absent) = %q, want empty", got)
	}
	mapped := tagsToMap(in)
	if mapped[tagDeployment] != "exasol-12345678" {
		t.Fatalf("tagsToMap missing Deployment: %v", mapped)
	}
}
