// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/exasol/exasol-personal/internal/approval"
	"github.com/exasol/exasol-personal/internal/localinstall"
	"github.com/exasol/exasol-personal/internal/localruntime"
	"github.com/spf13/cobra"
)

// hostRuntimePreparationOptions builds the policy the local runtime applies
// when it needs to change the host. Deciding here rather than inside the
// runtime keeps command presentation (prompt wording, --auto-approve, TTY
// detection) in the command layer.
func hostRuntimePreparationOptions(
	cmd *cobra.Command,
	mode approval.Mode,
) localruntime.PrepareOptions {
	return localruntime.PrepareOptions{
		ApproveHostChange:     hostChangeApprover(cmd, mode),
		ChooseContainerEngine: containerEngineChooser(cmd, mode),
		// Preparation progress goes to stderr unconditionally: it reports
		// multi-minute steps the user needs to see, and is not the
		// --verbose-gated subprocess output.
		Progress: cmd.ErrOrStderr(),
	}
}

// hostChangeApprover refuses rather than assumes when it cannot ask. A
// non-interactive run without --auto-approve fails with instructions instead
// of silently mutating the host.
func hostChangeApprover(
	cmd *cobra.Command,
	mode approval.Mode,
) localruntime.HostChangeApprover {
	return func(_ context.Context, request localruntime.HostChangeRequest) (bool, error) {
		switch mode {
		case approval.ModeApprove:
			return true, nil
		case approval.ModeNonInteractive:
			return false, fmt.Errorf(
				"local runtime host preparation requires approval: %s Re-run with "+
					"--auto-approve to install it%s",
				hostChangeReason(request), hostChangeAlternatives(request),
			)
		case approval.ModePrompt:
		default:
			return false, fmt.Errorf("unrecognised approval mode %q", mode)
		}

		writer := cmd.ErrOrStderr()
		_, _ = fmt.Fprintln(writer, hostChangeExplanation(request))
		if request.Alternative != "" {
			_, _ = fmt.Fprintf(writer, "To use %s instead, answer N and run %s.\n",
				containerEngineName(request.Alternative),
				selectContainerEngineCommand(request.Alternative))
		}
		_, _ = fmt.Fprintln(writer, "The launcher will run:")
		for _, command := range request.Commands {
			_, _ = fmt.Fprintf(writer, "  %s\n", command)
		}
		_, _ = fmt.Fprint(writer, "Continue? [y/N]: ")

		line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return false, fmt.Errorf("failed to read host preparation approval: %w", err)
		}

		return confirmYes(line), nil
	}
}

func hostChangeExplanation(request localruntime.HostChangeRequest) string {
	if request.Kind != localruntime.HostChangeInstallContainerRuntime {
		return hostChangeReason(request)
	}

	return hostChangeReason(request) + " Installing it may request administrator (UAC) " +
		"approval and take a few minutes."
}

// hostChangeReason names the deployment's engine, because the engine to
// install follows from the deployment's container_engine, not from what
// local deployments need in general.
func hostChangeReason(request localruntime.HostChangeRequest) string {
	if request.Kind != localruntime.HostChangeInstallContainerRuntime {
		return "The local runtime requires a host environment change."
	}

	return fmt.Sprintf("This deployment uses %s, but %s is not installed.",
		containerEngineName(request.Engine), containerEngineDisplayName(request.Engine))
}

// hostChangeAlternatives lists what an unattended run can do instead of
// approving: run the commands itself, or select an engine that can run here.
func hostChangeAlternatives(request localruntime.HostChangeRequest) string {
	var alternatives []string
	if len(request.Commands) > 0 {
		commands := make([]string, 0, len(request.Commands))
		for _, command := range request.Commands {
			commands = append(commands, "`"+command.String()+"`")
		}
		alternatives = append(alternatives, "run "+strings.Join(commands, " and ")+" yourself")
	}
	if request.Alternative != "" {
		alternatives = append(alternatives, "use "+containerEngineName(request.Alternative)+
			" with "+selectContainerEngineCommand(request.Alternative))
	}
	switch len(alternatives) {
	case 0:
		return ""
	case 1:
		return ", or " + alternatives[0]
	default:
		return ", " + alternatives[0] + ", or " + alternatives[1]
	}
}

func selectContainerEngineCommand(engine localinstall.ContainerEngine) string {
	return "`exasol config set --container-engine " + string(engine) + "`"
}

func containerEngineDisplayName(engine localinstall.ContainerEngine) string {
	if engine == localinstall.EngineDocker {
		return "Docker Desktop"
	}

	return "Podman"
}

func containerEngineName(engine localinstall.ContainerEngine) string {
	if engine == localinstall.EngineDocker {
		return "Docker"
	}

	return "Podman"
}

// containerEngineChooser asks which engine to install in one prompt. The
// answer approves the installation shown for that engine. --auto-approve
// takes the platform's preferred engine, which the request lists first.
func containerEngineChooser(
	cmd *cobra.Command,
	mode approval.Mode,
) localruntime.ContainerEngineChooser {
	return func(
		_ context.Context,
		request localruntime.ContainerEngineChoiceRequest,
	) (localinstall.ContainerEngine, error) {
		if len(request.Options) == 0 {
			return "", nil
		}
		switch mode {
		case approval.ModeApprove:
			return request.Options[0].Engine, nil
		case approval.ModeNonInteractive:
			return "", fmt.Errorf(
				"no container engine is installed for local deployments; re-run with "+
					"--auto-approve to install %s, or choose the engine to install with "+
					"`exasol config set --container-engine podman` or `docker` and re-run "+
					"with --auto-approve",
				containerEngineDisplayName(request.Options[0].Engine),
			)
		case approval.ModePrompt:
		default:
			return "", fmt.Errorf("unrecognised approval mode %q", mode)
		}

		writer := cmd.ErrOrStderr()
		_, _ = fmt.Fprintln(writer,
			"No container engine is installed. Local deployments need one, and the launcher "+
				"can install it through Windows Package Manager, which may request "+
				"administrator (UAC) approval and take a few minutes:")
		for index, option := range request.Options {
			_, _ = fmt.Fprintf(
				writer,
				"  %d) %s:",
				index+1,
				containerEngineDisplayName(option.Engine),
			)
			for _, command := range option.Commands {
				_, _ = fmt.Fprintf(writer, " %s", command)
			}
			_, _ = fmt.Fprintln(writer)
		}
		_, _ = fmt.Fprintf(writer, "Install which? [1-%d, Enter cancels]: ", len(request.Options))

		line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", fmt.Errorf("failed to read the container engine choice: %w", err)
		}

		return chosenContainerEngine(line, request.Options), nil
	}
}

// chosenContainerEngine accepts an option's number or engine name; anything
// else cancels.
func chosenContainerEngine(
	answer string,
	options []localruntime.ContainerEngineOption,
) localinstall.ContainerEngine {
	answer = strings.ToLower(strings.TrimSpace(answer))
	if number, err := strconv.Atoi(answer); err == nil && number >= 1 && number <= len(options) {
		return options[number-1].Engine
	}
	for _, option := range options {
		if answer != "" && answer == string(option.Engine) {
			return option.Engine
		}
	}

	return ""
}
