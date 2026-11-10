// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/exasol/exasol-personal/internal/approval"
	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/spf13/cobra"
)

const destroyCmdShortDesc = `Destroy a deployment`

// nolint: revive
const destroyCmdLongDesc = destroyCmdShortDesc + `

Destroying a deployment deletes all resources - including all data.
If you want to retain any data, make sure you've created and moved backups to another safe location.
By default, local deployment files are kept so the same deployment can be inspected or recreated.
Pass --remove to remove the local deployment directory after deployment resources have been destroyed.
`

var destroyOpts = struct {
	Remove bool
}{}

func registerDestroyFlags(cmd *cobra.Command) {
	registerVerboseFlag(cmd, commonFlags)
	// nolint: revive
	cmd.Flags().BoolVar(&destroyOpts.Remove,
		"remove",
		false,
		"Remove the local deployment directory after deployment resources are successfully destroyed")
}

var destroyCmd = &cobra.Command{
	Use:     "destroy",
	Short:   destroyCmdShortDesc,
	Long:    destroyCmdLongDesc,
	Args:    cobra.NoArgs,
	GroupID: rootCmdGroupLifecycle,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true

		deployment := commonFlags.Deployment()
		mode := rootOpts.ApprovalMode()

		proceed, err := confirmAction(
			mode,
			func() bool {
				removalTarget := ""
				if destroyOpts.Remove {
					removalTarget = deployment.Root()
				}

				return askForUserConfirmation(destroyConfirmationPrompt(removalTarget))
			},
		)
		if err != nil {
			return err
		}
		if !proceed {
			return nil
		}

		if err := deploy.Destroy(
			cmd.Context(),
			deployment,
			commonFlags.DeployVerbose,
			deploy.DestroyOptions{
				ConfirmWithoutContainerEngine: destroyWithoutContainerEngineConfirmer(cmd, mode),
			},
		); err != nil {
			return err
		}
		if destroyOpts.Remove {
			return deploy.RemoveLocalDeploymentDirectory(cmd.Context(), deployment)
		}

		return nil
	},
}

func destroyConfirmationPrompt(localRemovalTarget string) string {
	prompt := "WARNING: Destroying a deployment " +
		"is an irreversible operation, " +
		"and removes all deployment resources " +
		"- including all data."
	if localRemovalTarget != "" {
		prompt += "\n\nLocal deployment directory to remove after destroy:\n" + localRemovalTarget
	}
	prompt += "\n\nProceed with destroy? [y/N]"

	return prompt
}

// destroyWithoutContainerEngineConfirmer asks whether to destroy a deployment
// whose container engine command is gone. Only a person can accept, because
// the engine may still hold the deployment's containers; --auto-approve and
// runs without a terminal fail instead.
func destroyWithoutContainerEngineConfirmer(
	cmd *cobra.Command,
	mode approval.Mode,
) func(context.Context, deploy.MissingContainerEngine) (bool, error) {
	if mode != approval.ModePrompt {
		return nil
	}

	return func(_ context.Context, missing deploy.MissingContainerEngine) (bool, error) {
		name := containerEngineName(missing.Engine)
		command := string(missing.Engine)
		writer := cmd.ErrOrStderr()
		_, _ = fmt.Fprintf(writer, "%s is not installed, so the launcher cannot remove this "+
			"deployment's containers and network.\n", name)
		_, _ = fmt.Fprintf(writer, "Destroying without %s deletes the deployment's data and "+
			"files, and leaves anything %s still holds. If %s is installed again, remove "+
			"that with:\n", name, name, name)
		_, _ = fmt.Fprintf(writer, "  %s rm -f $(%s ps -aq --filter name=%s)\n",
			command, command, missing.Container)
		_, _ = fmt.Fprintf(writer, "  %s network rm %s\n", command, missing.Network)
		_, _ = fmt.Fprintf(writer, "Destroy without %s? [y/N]: ", name)

		line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return false, fmt.Errorf("failed to read the destroy answer: %w", err)
		}

		return confirmYes(line), nil
	}
}

// nolint: gochecknoinits
func init() {
	requireDefaultDeploymentCompatibility(destroyCmd)
	requireInitializedDeploymentDir(destroyCmd)
	requireDeploymentFileLogging(destroyCmd)
	registerDeploymentDirFlag(destroyCmd, commonFlags)
	registerDestroyFlags(destroyCmd)
	rootCmd.AddCommand(destroyCmd)
}
