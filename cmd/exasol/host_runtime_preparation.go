// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/exasol/exasol-personal/internal/localruntime"
	"github.com/exasol/exasol-personal/internal/util"
	"github.com/spf13/cobra"
)

func hostRuntimePreparationOptions(
	cmd *cobra.Command,
	autoApprove bool,
) localruntime.PrepareOptions {
	return localruntime.PrepareOptions{
		ApproveHostChange: hostChangeApprover(
			cmd, autoApprove, util.IsInteractiveStdin(),
		),
		Progress: cmd.ErrOrStderr(),
	}
}

//nolint:revive // autoApprove and interactive describe command presentation policy.
func hostChangeApprover(
	cmd *cobra.Command,
	autoApprove, interactive bool,
) localruntime.HostChangeApprover {
	return func(_ context.Context, request localruntime.HostChangeRequest) (bool, error) {
		if autoApprove {
			return true, nil
		}
		if !interactive {
			return false, errors.New(
				"local runtime host preparation requires approval; " +
					"re-run with --auto-approve or run the displayed setup manually",
			)
		}

		writer := cmd.ErrOrStderr()
		_, _ = fmt.Fprintln(writer, hostChangeExplanation(request.Kind))
		_, _ = fmt.Fprintln(writer, "The launcher will run:")
		for _, command := range request.Commands {
			_, _ = fmt.Fprintf(writer, "  %s\n", formatDisplayedHostCommand(command))
		}
		_, _ = fmt.Fprint(writer, "Continue? [y/N]: ")

		line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return false, fmt.Errorf("failed to read host preparation approval: %w", err)
		}

		return confirmYes(line), nil
	}
}

func hostChangeExplanation(kind localruntime.HostChangeKind) string {
	switch kind {
	case localruntime.HostChangeInstallContainerRuntime:
		return "Podman is required for local deployments but is not available."
	case localruntime.HostChangeEnablePrivilegedRuntime:
		return "The default Podman machine must be converted to rootful mode."
	default:
		return "The local runtime requires a host environment change."
	}
}

func formatDisplayedHostCommand(command localruntime.HostCommand) string {
	result := command.Name
	for _, argument := range command.Args {
		result += " " + argument
	}

	return result
}
