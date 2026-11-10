// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/approval"
	"github.com/exasol/exasol-personal/internal/localinstall"
	"github.com/exasol/exasol-personal/internal/localruntime"
	"github.com/spf13/cobra"
)

func newApproverTestCommand(stdin string) (*cobra.Command, *bytes.Buffer) {
	cmd := &cobra.Command{Use: "test"}
	errOut := &bytes.Buffer{}
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetErr(errOut)

	return cmd, errOut
}

var testHostChangeRequest = localruntime.HostChangeRequest{
	Kind: localruntime.HostChangeInstallContainerRuntime,
	Commands: []localruntime.HostCommand{
		{Name: "winget", Args: []string{"install", "--exact", "--id", "RedHat.Podman"}},
	},
}

func TestHostChangeApprover_AutoApproveSkipsPrompt(t *testing.T) {
	t.Parallel()

	cmd, errOut := newApproverTestCommand("")
	approver := hostChangeApprover(cmd, approval.ModeApprove)

	approved, err := approver(context.Background(), testHostChangeRequest)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !approved {
		t.Error("expected --auto-approve to approve the change")
	}
	if errOut.Len() != 0 {
		t.Errorf("expected no prompt output with --auto-approve, got %q", errOut.String())
	}
}

// The behaviour this replaces silently defaulted to "yes" whenever stdin was
// not a terminal, so a scripted run installed Podman with no approval. A
// non-interactive run must now refuse and say how to proceed.
func TestHostChangeApprover_NonInteractiveRefusesWithoutAutoApprove(t *testing.T) {
	t.Parallel()

	cmd, _ := newApproverTestCommand("")
	approver := hostChangeApprover(cmd, approval.ModeNonInteractive)

	approved, err := approver(context.Background(), testHostChangeRequest)
	if err == nil {
		t.Fatal("expected a non-interactive run without --auto-approve to fail")
	}
	if approved {
		t.Error("expected the change to be denied")
	}
	if !strings.Contains(err.Error(), "--auto-approve") {
		t.Errorf("expected the error to name --auto-approve, got %v", err)
	}
}

func TestHostChangeApprover_InteractiveShowsCommandsAndAcceptsYes(t *testing.T) {
	t.Parallel()

	cmd, errOut := newApproverTestCommand("y\n")
	approver := hostChangeApprover(cmd, approval.ModePrompt)

	approved, err := approver(context.Background(), testHostChangeRequest)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !approved {
		t.Error("expected 'y' to approve the change")
	}
	// The user must see the exact command before deciding.
	if !strings.Contains(errOut.String(), "winget install --exact --id RedHat.Podman") {
		t.Errorf("expected the command to be displayed, got %q", errOut.String())
	}
	if !strings.Contains(
		errOut.String(),
		"This deployment uses Podman, but Podman is not installed",
	) {
		t.Errorf("expected an explanation of the change, got %q", errOut.String())
	}
}

func TestHostChangeApprover_InteractiveDefaultsToNo(t *testing.T) {
	t.Parallel()

	for _, input := range []string{"\n", "n\n", "no\n", ""} {
		cmd, _ := newApproverTestCommand(input)
		approver := hostChangeApprover(cmd, approval.ModePrompt)

		approved, err := approver(context.Background(), testHostChangeRequest)
		if err != nil {
			t.Fatalf("input %q: unexpected error: %v", input, err)
		}
		if approved {
			t.Errorf("input %q: expected the change to be declined by default", input)
		}
	}
}

func TestHostRuntimePreparationOptions_AlwaysSuppliesApproverAndProgress(t *testing.T) {
	t.Parallel()

	cmd, errOut := newApproverTestCommand("")
	options := hostRuntimePreparationOptions(cmd, approval.ModeApprove)

	if options.ApproveHostChange == nil {
		t.Error("expected an approver to always be supplied")
	}
	// Progress must not be gated on --verbose: it reports multi-minute steps.
	if options.Progress == nil {
		t.Fatal("expected a progress writer")
	}
	if _, err := options.Progress.Write([]byte("preparing")); err != nil {
		t.Fatalf("progress write failed: %v", err)
	}
	if errOut.String() != "preparing" {
		t.Errorf("expected progress on stderr, got %q", errOut.String())
	}
}

var testEngineChoiceRequest = localruntime.ContainerEngineChoiceRequest{
	Options: []localruntime.ContainerEngineOption{
		{
			Engine: localinstall.EnginePodman,
			Commands: []localruntime.HostCommand{
				{Name: "winget", Args: []string{"install", "--exact", "--id", "RedHat.Podman"}},
			},
		},
		{
			Engine: localinstall.EngineDocker,
			Commands: []localruntime.HostCommand{
				{
					Name: "winget",
					Args: []string{"install", "--exact", "--id", "Docker.DockerDesktop"},
				},
			},
		},
	},
}

func TestContainerEngineChooser_InteractiveShowsBothEnginesInOnePrompt(t *testing.T) {
	t.Parallel()

	for input, want := range map[string]localinstall.ContainerEngine{
		"1\n":      localinstall.EnginePodman,
		"2\n":      localinstall.EngineDocker,
		"Docker\n": localinstall.EngineDocker,
		"podman":   localinstall.EnginePodman,
		"\n":       "",
		"3\n":      "",
		"y\n":      "",
		"":         "",
	} {
		cmd, errOut := newApproverTestCommand(input)
		chooser := containerEngineChooser(cmd, approval.ModePrompt)

		chosen, err := chooser(context.Background(), testEngineChoiceRequest)
		if err != nil {
			t.Fatalf("input %q: unexpected error: %v", input, err)
		}
		if chosen != want {
			t.Errorf("input %q: expected %q, got %q", input, want, chosen)
		}
		prompt := errOut.String()
		if !strings.Contains(prompt, "1) Podman: winget install --exact --id RedHat.Podman") ||
			!strings.Contains(
				prompt,
				"2) Docker Desktop: winget install --exact --id Docker.DockerDesktop",
			) ||
			strings.Count(prompt, "Install which?") != 1 {
			t.Errorf("input %q: expected one prompt listing both commands, got %q", input, prompt)
		}
	}
}

func TestContainerEngineChooser_AutoApproveChoosesThePreferredEngine(t *testing.T) {
	t.Parallel()

	cmd, errOut := newApproverTestCommand("")
	chosen, err := containerEngineChooser(cmd, approval.ModeApprove)(
		context.Background(), testEngineChoiceRequest,
	)
	if err != nil || chosen != localinstall.EnginePodman {
		t.Fatalf("expected Podman under --auto-approve, got %q %v", chosen, err)
	}
	if errOut.Len() != 0 {
		t.Errorf("expected no prompt with --auto-approve, got %q", errOut.String())
	}
}

func TestContainerEngineChooser_NonInteractiveNamesTheFlags(t *testing.T) {
	t.Parallel()

	cmd, _ := newApproverTestCommand("")
	chosen, err := containerEngineChooser(cmd, approval.ModeNonInteractive)(
		context.Background(), testEngineChoiceRequest,
	)
	if err == nil || chosen != "" {
		t.Fatalf("expected a failure without a terminal, got %q %v", chosen, err)
	}
	for _, flag := range []string{"--auto-approve", "exasol config set --container-engine"} {
		if !strings.Contains(err.Error(), flag) {
			t.Errorf("expected the error to name %s, got %v", flag, err)
		}
	}
}

func TestHostChangeApprover_NamesTheEngineItInstalls(t *testing.T) {
	t.Parallel()

	cmd, errOut := newApproverTestCommand("y\n")
	request := localruntime.HostChangeRequest{
		Kind:        localruntime.HostChangeInstallContainerRuntime,
		Engine:      localinstall.EngineDocker,
		Alternative: localinstall.EnginePodman,
	}
	if _, err := hostChangeApprover(cmd, approval.ModePrompt)(
		context.Background(), request,
	); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, part := range []string{
		"This deployment uses Docker, but Docker Desktop is not installed.",
		"To use Podman instead, answer N and run `exasol config set --container-engine podman`.",
	} {
		if !strings.Contains(errOut.String(), part) {
			t.Errorf("expected %q in the prompt, got %q", part, errOut.String())
		}
	}
	_, err := hostChangeApprover(cmd, approval.ModeNonInteractive)(context.Background(), request)
	for _, part := range []string{
		"This deployment uses Docker, but Docker Desktop is not installed.",
		"Re-run with --auto-approve to install it",
		"use Podman with `exasol config set --container-engine podman`",
	} {
		if err == nil || !strings.Contains(err.Error(), part) {
			t.Errorf("expected %q in the unattended failure, got %v", part, err)
		}
	}
}

// Docker that runs Windows containers cannot replace a missing Podman, so the
// request carries no alternative and nothing suggests Docker.
func TestHostChangeApprover_SuggestsNoAlternativeThatCannotRun(t *testing.T) {
	t.Parallel()

	cmd, errOut := newApproverTestCommand("n\n")
	request := localruntime.HostChangeRequest{
		Kind:   localruntime.HostChangeInstallContainerRuntime,
		Engine: localinstall.EnginePodman,
	}
	if _, err := hostChangeApprover(cmd, approval.ModePrompt)(
		context.Background(), request,
	); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err := hostChangeApprover(cmd, approval.ModeNonInteractive)(context.Background(), request)
	if strings.Contains(errOut.String(), "--container-engine") ||
		err == nil || strings.Contains(err.Error(), "--container-engine") {
		t.Errorf("expected no engine to be suggested, got %q and %v", errOut.String(), err)
	}
}

var testEngineOffer = localruntime.ContainerEngineOffer{
	Unusable: localinstall.EngineDocker,
	Cause:    errors.New("docker is not available: the daemon does not answer"),
	Offer: localruntime.ContainerEngineOption{
		Engine: localinstall.EnginePodman,
		Commands: []localruntime.HostCommand{
			{Name: "winget", Args: []string{"install", "--exact", "--id", "RedHat.Podman"}},
		},
	},
}

func TestContainerEngineOfferer_InteractiveStatesWhyAndShowsTheInstallation(t *testing.T) {
	t.Parallel()

	answers := map[string]bool{"y\n": true, "yes\n": true, "\n": false, "n\n": false, "": false}
	for input, want := range answers {
		cmd, errOut := newApproverTestCommand(input)

		accepted, err := containerEngineOfferer(cmd, approval.ModePrompt)(
			context.Background(), testEngineOffer,
		)
		if err != nil {
			t.Fatalf("input %q: unexpected error: %v", input, err)
		}
		if accepted != want {
			t.Errorf("input %q: expected %v, got %v", input, want, accepted)
		}
		prompt := errOut.String()
		for _, part := range []string{
			"Docker cannot be used: docker is not available: the daemon does not answer",
			"can run on Podman instead",
			"winget install --exact --id RedHat.Podman",
			"Use Podman? [y/N]",
		} {
			if !strings.Contains(prompt, part) {
				t.Errorf("input %q: expected %q in the prompt, got %q", input, part, prompt)
			}
		}
	}
}

func TestContainerEngineOfferer_OnlyAPersonCanAccept(t *testing.T) {
	t.Parallel()

	for _, mode := range []approval.Mode{approval.ModeApprove, approval.ModeNonInteractive} {
		cmd, errOut := newApproverTestCommand("y\n")

		accepted, err := containerEngineOfferer(cmd, mode)(context.Background(), testEngineOffer)
		if err != nil || accepted {
			t.Errorf("mode %q: expected the offer to be declined, got %v %v", mode, accepted, err)
		}
		if errOut.Len() != 0 {
			t.Errorf("mode %q: expected no prompt, got %q", mode, errOut.String())
		}
	}
}
