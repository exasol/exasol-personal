// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestDeploymentDirFlagUnknownOnCommandWithoutFlag(t *testing.T) {
	t.Parallel()
	root := &cobra.Command{Use: "exasol"}
	versionCmd := &cobra.Command{
		Use:  "version",
		RunE: func(*cobra.Command, []string) error { return nil },
	}
	root.AddCommand(versionCmd)

	root.SetArgs([]string{"version", "--deployment-dir", "/tmp"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unknown flag") ||
		!strings.Contains(err.Error(), "deployment-dir") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeploymentDirFlagAcceptedAndAbsolutizedOnCommandWithFlag(t *testing.T) {
	t.Parallel()
	root := &cobra.Command{Use: "exasol"}
	state := &CommonFlags{}

	deployCmd := &cobra.Command{Use: "deploy", RunE: func(*cobra.Command, []string) error {
		if !filepath.IsAbs(state.DeploymentDir) {
			return errors.New("deployment dir is not absolute")
		}

		return nil
	}}
	registerDeploymentDirFlag(deployCmd, state)
	root.AddCommand(deployCmd)

	root.SetArgs([]string{"deploy", "--deployment-dir", "."})
	if err := root.Execute(); err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
}
