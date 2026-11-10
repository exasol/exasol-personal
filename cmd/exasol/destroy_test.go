// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/approval"
	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/exasol/exasol-personal/internal/localinstall"
)

var testMissingEngine = deploy.MissingContainerEngine{
	Engine:    localinstall.EngineDocker,
	Container: "exasol-db-2387318e",
	Network:   "exasol-db-2387318e-services",
}

func TestDestroyWithoutContainerEngineConfirmer_NamesWhatDockerKeeps(t *testing.T) {
	t.Parallel()

	answers := map[string]bool{"y\n": true, "yes\n": true, "\n": false, "n\n": false, "": false}
	for input, want := range answers {
		cmd, errOut := newApproverTestCommand(input)

		confirmed, err := destroyWithoutContainerEngineConfirmer(cmd, approval.ModePrompt)(
			context.Background(), testMissingEngine,
		)
		if err != nil {
			t.Fatalf("input %q: unexpected error: %v", input, err)
		}
		if confirmed != want {
			t.Errorf("input %q: expected %v, got %v", input, want, confirmed)
		}
		prompt := errOut.String()
		for _, part := range []string{
			"Docker is not installed",
			"docker rm -f $(docker ps -aq --filter name=exasol-db-2387318e)",
			"docker network rm exasol-db-2387318e-services",
			"Destroy without Docker? [y/N]",
		} {
			if !strings.Contains(prompt, part) {
				t.Errorf("input %q: expected %q in the prompt, got %q", input, part, prompt)
			}
		}
	}
}

func TestDestroyWithoutContainerEngineConfirmer_OnlyAPersonIsAsked(t *testing.T) {
	t.Parallel()

	for _, mode := range []approval.Mode{approval.ModeApprove, approval.ModeNonInteractive} {
		cmd, _ := newApproverTestCommand("y\n")
		if destroyWithoutContainerEngineConfirmer(cmd, mode) != nil {
			t.Errorf("mode %q: expected no confirmer, so the destroy fails", mode)
		}
	}
}
