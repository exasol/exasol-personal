// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/localruntime"
	"github.com/spf13/cobra"
)

func TestHostChangeApprover_AutoApprovesWithoutPrompt(t *testing.T) {
	t.Parallel()
	cmd := &cobra.Command{}
	output := &bytes.Buffer{}
	cmd.SetErr(output)
	approver := hostChangeApprover(cmd, true, false)

	approved, err := approver(context.Background(), localruntime.HostChangeRequest{
		Kind: localruntime.HostChangeInstallContainerRuntime,
	})

	if err != nil || !approved {
		t.Fatalf("expected automatic approval, approved=%t err=%v", approved, err)
	}
	if output.Len() != 0 {
		t.Fatalf("expected no prompt, got %q", output.String())
	}
}

func TestHostChangeApprover_NonInteractiveRequiresAutoApprove(t *testing.T) {
	t.Parallel()
	approver := hostChangeApprover(&cobra.Command{}, false, false)

	approved, err := approver(context.Background(), localruntime.HostChangeRequest{})

	if err == nil || !strings.Contains(err.Error(), "--auto-approve") || approved {
		t.Fatalf("expected actionable non-interactive refusal, approved=%t err=%v", approved, err)
	}
}

func TestHostChangeApprover_DisplaysExactCommandsOnStderr(t *testing.T) {
	t.Parallel()
	cmd := &cobra.Command{}
	output := &bytes.Buffer{}
	cmd.SetErr(output)
	cmd.SetIn(strings.NewReader("yes\n"))
	approver := hostChangeApprover(cmd, false, true)
	request := localruntime.HostChangeRequest{
		Kind: localruntime.HostChangeInstallContainerRuntime,
		Commands: []localruntime.HostCommand{{
			Name: "runtime-tool", Args: []string{"install", "container-runtime"},
		}},
	}

	approved, err := approver(context.Background(), request)

	if err != nil || !approved {
		t.Fatalf("expected interactive approval, approved=%t err=%v", approved, err)
	}
	if !strings.Contains(output.String(), "runtime-tool install container-runtime") ||
		!strings.Contains(output.String(), "Continue? [y/N]") {
		t.Fatalf("expected exact command prompt on stderr, got %q", output.String())
	}
}
