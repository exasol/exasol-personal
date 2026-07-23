// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/exasol/exasol-personal/internal/util"
	"github.com/spf13/cobra"
)

var slcInstallOpts = struct {
	AutoApprove bool
	NoRestart   bool
}{}

var slcUpdateOpts = struct {
	AutoApprove bool
	NoRestart   bool
}{}

var slcRemoveOpts = struct {
	AutoApprove bool
	NoRestart   bool
}{}

const slcOperationAbortedMessage = "Aborted; no changes were made."

const slcAutoApproveFlagName = "auto-approve"

const slcAutoApproveFlagDesc = "Do not prompt for confirmation before restarting the database"

const slcNoRestartFlagName = "no-restart"

const slcCmdShortDesc = "Manage script language containers (SLCs)"

const slcNoneAvailableText = "No script language containers are available for this platform."

const slcCmdLongDesc = slcCmdShortDesc + `

Script language containers provide the language runtimes used by UDFs.
Local deployments ship without any SLC installed; install one to run UDFs in that language.
`

var slcCmd = &cobra.Command{
	Use:   "slc",
	Short: slcCmdShortDesc,
	Long:  slcCmdLongDesc,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return cmd.Help()
	},
}

var slcInstallCmd = &cobra.Command{
	Use:   "install <alias>",
	Short: "Install an official script language container",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		alias := args[0]
		result, err := deploy.InstallSLC(
			cmd.Context(),
			commonFlags.Deployment(),
			alias,
			commonFlags.DeployVerbose,
			!slcInstallOpts.NoRestart,
			slcConfirmFunc(cmd, slcInstallOpts.AutoApprove, fmt.Sprintf("Installing %q", alias)),
		)
		if errors.Is(err, deploy.ErrSLCOperationCancelled) {
			addTerminalNotice(slcOperationAbortedMessage)

			return nil
		}
		if err != nil {
			return err
		}

		if commonFlags.OutputJson {
			return renderSLCCommandJSON(result)
		}

		if result.AlreadyInstalled {
			addTerminalOutput(fmt.Sprintf(
				"%s is already installed and up to date (version %s, aliases: %s). Nothing to do.",
				result.Entry.Flavor,
				result.Entry.Version,
				strings.Join(result.Entry.Aliases, ", "),
			))

			return nil
		}

		printSLCInstallResult(result)

		return nil
	},
}

var slcUpdateCmd = &cobra.Command{
	Use:   "update <alias>",
	Short: "Update an installed script language container to the catalog's current version",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		alias := args[0]
		result, err := deploy.UpdateSLC(
			cmd.Context(),
			commonFlags.Deployment(),
			alias,
			commonFlags.DeployVerbose,
			!slcUpdateOpts.NoRestart,
			slcConfirmFunc(cmd, slcUpdateOpts.AutoApprove, fmt.Sprintf("Updating %q", alias)),
		)
		if errors.Is(err, deploy.ErrSLCOperationCancelled) {
			addTerminalNotice(slcOperationAbortedMessage)

			return nil
		}
		if err != nil {
			return err
		}

		if commonFlags.OutputJson {
			return renderSLCCommandJSON(result)
		}

		if !result.Found {
			addTerminalOutput(fmt.Sprintf(
				"No SLC matching %q is installed, so there is nothing to update.",
				alias,
			))
			addTerminalCallToAction(
				fmt.Sprintf("Run `exasol slc install %s` to install it.", alias),
			)

			return nil
		}
		if result.Unchanged {
			addTerminalOutput(fmt.Sprintf(
				"%s is already up to date (version %s). Nothing to do.",
				result.Entry.Flavor,
				result.Entry.Version,
			))

			return nil
		}

		printSLCUpdateResult(result)

		return nil
	},
}

var slcRemoveCmd = &cobra.Command{
	Use:   "remove <alias>",
	Short: "Remove a script language container",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		alias := args[0]
		result, err := deploy.RemoveSLC(
			cmd.Context(),
			commonFlags.Deployment(),
			alias,
			commonFlags.DeployVerbose,
			!slcRemoveOpts.NoRestart,
			slcConfirmFunc(cmd, slcRemoveOpts.AutoApprove, fmt.Sprintf("Removing %q", alias)),
		)
		if errors.Is(err, deploy.ErrSLCOperationCancelled) {
			addTerminalNotice(slcOperationAbortedMessage)

			return nil
		}
		if err != nil {
			return err
		}

		if commonFlags.OutputJson {
			return renderSLCCommandJSON(result)
		}

		if !result.Found {
			addTerminalOutput(fmt.Sprintf(
				"No SLC matching %q is installed, so there is nothing to remove.",
				alias,
			))

			return nil
		}

		printSLCRemoveResult(result)

		return nil
	},
}

// slcConfirmFunc returns a confirmation callback for a database-restarting SLC operation.
// It returns nil when the user passed --auto-approve (pre-approved). Otherwise it warns and
// prompts interactively, and refuses non-interactively so scripts never trigger a silent
// restart.
//
//nolint:revive // autoApprove reflects the user's flag, not internal control coupling.
func slcConfirmFunc(cmd *cobra.Command, autoApprove bool, action string) deploy.ConfirmFunc {
	if autoApprove {
		return nil
	}

	return func() (bool, error) {
		if !util.IsInteractiveStdin() {
			return false, errors.New(
				"this restarts the database; re-run with --auto-approve to confirm, " +
					"or --no-restart to apply on the next start",
			)
		}

		_, _ = fmt.Fprintf(
			cmd.ErrOrStderr(),
			"%s will restart the database. Open connections will be dropped and running "+
				"statements aborted.\nContinue? [y/N]: ",
			action,
		)

		line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return false, err
		}

		return confirmYes(line), nil
	}
}

var slcListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available script language containers and which are installed",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
		statuses, err := deploy.SLCStatuses(commonFlags.Deployment())
		if err != nil {
			return err
		}

		if commonFlags.OutputJson {
			return queueSLCListJSON(statuses)
		}

		renderSLCListText(statuses)

		return nil
	},
}

func renderSLCListJSON(writer io.Writer, statuses []deploy.SLCStatus) error {
	content, err := formatSLCListJSON(statuses)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(writer, content)

	return err
}

func queueSLCListJSON(statuses []deploy.SLCStatus) error {
	content, err := formatSLCListJSON(statuses)
	if err != nil {
		return err
	}
	addTerminalOutput(content)

	return nil
}

func formatSLCListJSON(statuses []deploy.SLCStatus) (string, error) {
	content, err := json.MarshalIndent(statuses, "", "  ")
	if err != nil {
		return "", err
	}

	return string(content), nil
}

func renderSLCListText(statuses []deploy.SLCStatus) {
	addTerminalOutput(formatSLCListText(statuses))
}

func formatSLCListText(statuses []deploy.SLCStatus) string {
	if len(statuses) == 0 {
		return slcNoneAvailableText
	}

	var builder strings.Builder
	const columnPadding = 2
	writer := tabwriter.NewWriter(&builder, 0, 0, columnPadding, ' ', 0)
	_, _ = fmt.Fprintln(writer, "FLAVOR\tALIASES\tVERSION\tINSTALLED")
	for _, status := range statuses {
		installed := "no"
		if status.Installed {
			installed = "yes"
		}
		_, _ = fmt.Fprintf(
			writer,
			"%s\t%s\t%s\t%s\n",
			status.Flavor,
			strings.Join(status.Aliases, ", "),
			status.Version,
			installed,
		)
	}
	_ = writer.Flush()

	return strings.TrimRight(builder.String(), "\n")
}

func printSLCInstallResult(result *deploy.SLCInstallResult) {
	verb := "Installed"
	if result.Replaced {
		verb = "Updated"
	}
	message := fmt.Sprintf(
		"%s %s (aliases: %s).",
		verb,
		result.Entry.Flavor,
		strings.Join(result.Entry.Aliases, ", "),
	)

	switch result.Outcome {
	case deploy.SLCApplyNone:
	case deploy.SLCApplyRestarted:
		message += " Database restarted; the language is ready to use."
	case deploy.SLCApplyStarted:
		message += " Database started; the language is ready to use."
	case deploy.SLCApplyDeferred:
		message += " It will become available on the next start."
	default:
	}

	addTerminalOutput(message)
}

func printSLCUpdateResult(result *deploy.SLCUpdateResult) {
	message := fmt.Sprintf("Updated %s.", result.Entry.Flavor)
	if result.FromFlavor != "" && result.FromFlavor != result.Entry.Flavor {
		message = fmt.Sprintf("Updated %s to %s.", result.FromFlavor, result.Entry.Flavor)
	}

	switch result.Outcome {
	case deploy.SLCApplyNone:
	case deploy.SLCApplyRestarted:
		message += " Database restarted; the new version is ready to use."
	case deploy.SLCApplyStarted:
		message += " Database started; the new version is ready to use."
	case deploy.SLCApplyDeferred:
		message += " It will take effect on the next start."
	default:
	}

	addTerminalOutput(message)
}

func printSLCRemoveResult(result *deploy.SLCRemoveResult) {
	message := fmt.Sprintf("Removed %s.", result.Entry.Flavor)

	switch result.Outcome {
	case deploy.SLCApplyNone:
	case deploy.SLCApplyRestarted:
		message += " Database restarted; the language is no longer available."
	case deploy.SLCApplyStarted:
		message += " Database started."
	case deploy.SLCApplyDeferred:
		message += " It will no longer be loaded on the next start."
	default:
	}

	addTerminalOutput(message)
}

func renderSLCCommandJSON(value any) error {
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	addTerminalOutput(string(content))

	return nil
}

// nolint: gochecknoinits
func init() {
	requireDefaultDeploymentCompatibility(slcInstallCmd)
	requireInitializedDeploymentDir(slcInstallCmd)
	registerDeploymentDirFlag(slcInstallCmd, commonFlags)
	registerOutputFlags(slcInstallCmd, commonFlags)
	registerVerboseFlag(slcInstallCmd, commonFlags)
	slcInstallCmd.Flags().BoolVar(&slcInstallOpts.AutoApprove, slcAutoApproveFlagName, false,
		slcAutoApproveFlagDesc)
	slcInstallCmd.Flags().BoolVar(&slcInstallOpts.NoRestart, slcNoRestartFlagName, false,
		"Record the SLC without restarting; it activates on the next start")

	requireDefaultDeploymentCompatibility(slcUpdateCmd)
	requireInitializedDeploymentDir(slcUpdateCmd)
	registerDeploymentDirFlag(slcUpdateCmd, commonFlags)
	registerOutputFlags(slcUpdateCmd, commonFlags)
	registerVerboseFlag(slcUpdateCmd, commonFlags)
	slcUpdateCmd.Flags().BoolVar(&slcUpdateOpts.AutoApprove, slcAutoApproveFlagName, false,
		slcAutoApproveFlagDesc)
	slcUpdateCmd.Flags().BoolVar(&slcUpdateOpts.NoRestart, slcNoRestartFlagName, false,
		"Record the update without restarting; it applies on the next start")

	requireDefaultDeploymentCompatibility(slcRemoveCmd)
	requireInitializedDeploymentDir(slcRemoveCmd)
	registerDeploymentDirFlag(slcRemoveCmd, commonFlags)
	registerOutputFlags(slcRemoveCmd, commonFlags)
	registerVerboseFlag(slcRemoveCmd, commonFlags)
	slcRemoveCmd.Flags().BoolVar(&slcRemoveOpts.AutoApprove, slcAutoApproveFlagName, false,
		slcAutoApproveFlagDesc)
	slcRemoveCmd.Flags().BoolVar(&slcRemoveOpts.NoRestart, slcNoRestartFlagName, false,
		"Record the removal without restarting; it applies on the next start")

	registerDeploymentDirFlag(slcListCmd, commonFlags)
	registerOutputFlags(slcListCmd, commonFlags)

	slcCmd.AddCommand(slcInstallCmd)
	slcCmd.AddCommand(slcListCmd)
	slcCmd.AddCommand(slcUpdateCmd)
	slcCmd.AddCommand(slcRemoveCmd)
	rootCmd.AddCommand(slcCmd)
}
