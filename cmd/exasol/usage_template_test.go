// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// nolint: paralleltest
func TestInfrastructureVariableFlagsTitle_IncludesPresetLabel(t *testing.T) {
	// Do not run in parallel: this test temporarily modifies global infraFlagToVarName.
	oldMap := infraFlagToVarName
	infraFlagToVarName = map[string]string{"dummy-infra": "dummy_infra"}
	t.Cleanup(func() { infraFlagToVarName = oldMap })

	cmd := &cobra.Command{Use: "init"}
	cmd.Flags().String("dummy-infra", "", "dummy infra")
	cmd.Annotations = map[string]string{infraPresetLabelAnnotationKey: "aws"}
	cmd.SetUsageTemplate(customUsageTemplate)

	usage := cmd.UsageString()
	if !strings.Contains(usage, "Infrastructure variable flags of preset `aws`:") {
		t.Fatalf("expected usage to contain preset label header, got:\n%s", usage)
	}
}

// nolint: paralleltest
func TestInfrastructureVariableFlagsTitle_FallsBackWhenLabelUnknown(t *testing.T) {
	// Do not run in parallel: this test temporarily modifies global infraFlagToVarName.
	oldMap := infraFlagToVarName
	infraFlagToVarName = map[string]string{"dummy-infra": "dummy_infra"}
	t.Cleanup(func() { infraFlagToVarName = oldMap })

	cmd := &cobra.Command{Use: "init"}
	cmd.Flags().String("dummy-infra", "", "dummy infra")
	cmd.SetUsageTemplate(customUsageTemplate)

	usage := cmd.UsageString()
	if !strings.Contains(usage, "Infrastructure variable flags:") {
		t.Fatalf("expected usage to contain fallback infra flags header, got:\n%s", usage)
	}
	if strings.Contains(usage, "Infrastructure variable flags of preset") {
		t.Fatalf("did not expect usage to contain preset label header, got:\n%s", usage)
	}
}

// nolint: paralleltest
func TestInstallationVariableFlagsTitle_IncludesPresetLabel(t *testing.T) {
	// Do not run in parallel: this test temporarily modifies global installFlagToVarName.
	oldMap := installFlagToVarName
	installFlagToVarName = map[string]string{"dummy-install": "dummy_install"}
	t.Cleanup(func() { installFlagToVarName = oldMap })

	cmd := &cobra.Command{Use: "init"}
	cmd.Flags().String("dummy-install", "", "dummy install")
	cmd.Annotations = map[string]string{installPresetLabelAnnotationKey: "ubuntu"}
	cmd.SetUsageTemplate(customUsageTemplate)

	usage := cmd.UsageString()
	if !strings.Contains(usage, "Installation variable flags of preset `ubuntu`:") {
		t.Fatalf("expected usage to contain preset label header, got:\n%s", usage)
	}
}

// nolint: paralleltest
func TestInstallationVariableFlagsTitle_FallsBackWhenLabelUnknown(t *testing.T) {
	// Do not run in parallel: this test temporarily modifies global installFlagToVarName.
	oldMap := installFlagToVarName
	installFlagToVarName = map[string]string{"dummy-install": "dummy_install"}
	t.Cleanup(func() { installFlagToVarName = oldMap })

	cmd := &cobra.Command{Use: "init"}
	cmd.Flags().String("dummy-install", "", "dummy install")
	cmd.SetUsageTemplate(customUsageTemplate)

	usage := cmd.UsageString()
	if !strings.Contains(usage, "Installation variable flags:") {
		t.Fatalf("expected usage to contain fallback install flags header, got:\n%s", usage)
	}
	if strings.Contains(usage, "Installation variable flags of preset") {
		t.Fatalf("did not expect usage to contain preset label header, got:\n%s", usage)
	}
}
