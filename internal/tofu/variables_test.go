// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package tofu

import (
	"testing"

	"github.com/zclconf/go-cty/cty"
)

func TestParseVarsValuesFileReturnsTopLevelAssignedValues(t *testing.T) {
	t.Parallel()

	content := `
region = "eu-central-1"
instance_count = 3
enabled = true
`

	values, err := ParseVarsValuesFile([]byte(content), "vars.tfvars")
	if err != nil {
		t.Fatalf("expected parsing to succeed, got %v", err)
	}

	if !values["region"].RawEquals(cty.StringVal("eu-central-1")) {
		t.Fatalf("expected region to be eu-central-1, got %#v", values["region"])
	}
	if !values["instance_count"].RawEquals(cty.NumberIntVal(3)) {
		t.Fatalf("expected instance_count to be 3, got %#v", values["instance_count"])
	}
	if !values["enabled"].RawEquals(cty.True) {
		t.Fatalf("expected enabled to be true, got %#v", values["enabled"])
	}
}

func TestParseVarsValuesFileRejectsInvalidHCL(t *testing.T) {
	t.Parallel()

	_, err := ParseVarsValuesFile([]byte("region = "), "vars.tfvars")
	if err == nil {
		t.Fatal("expected an error for malformed HCL, got nil")
	}
}
