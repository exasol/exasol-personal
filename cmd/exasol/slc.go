// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/exasol/exasol-personal/internal/util"
	"github.com/spf13/cobra"
)

var slcInstallOpts = struct {
	Yes       bool
	NoRestart bool
}{}

var slcUpdateOpts = struct {
	Yes       bool
	NoRestart bool
}{}

var slcRemoveOpts = struct {
	Yes       bool
	NoRestart bool
}{}

const slcCmdShortDesc = "Manage script language containers (SLCs)"

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
			slcConfirmFunc(cmd, slcInstallOpts.Yes, fmt.Sprintf("Installing %q", alias)),
		)
		if errors.Is(err, deploy.ErrSLCOperationCancelled) {
			safePrint("Aborted; no changes were made.")

			return nil
		}
		if err != nil {
			return err
		}

		if result.AlreadyInstalled {
			safePrint(fmt.Sprintf(
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
			slcConfirmFunc(cmd, slcUpdateOpts.Yes, fmt.Sprintf("Updating %q", alias)),
		)
		if errors.Is(err, deploy.ErrSLCOperationCancelled) {
			safePrint("Aborted; no changes were made.")

			return nil
		}
		if err != nil {
			return err
		}

		if !result.Found {
			safePrint(fmt.Sprintf(
				"No SLC matching %q is installed, so there is nothing to update.\n"+
					"Run `exasol slc install %s` to install it.",
				alias, alias,
			))

			return nil
		}
		if result.Unchanged {
			safePrint(fmt.Sprintf(
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
			slcConfirmFunc(cmd, slcRemoveOpts.Yes, fmt.Sprintf("Removing %q", alias)),
		)
		if errors.Is(err, deploy.ErrSLCOperationCancelled) {
			safePrint("Aborted; no changes were made.")

			return nil
		}
		if err != nil {
			return err
		}

		if !result.Found {
			safePrint(fmt.Sprintf(
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
// It returns nil when the user passed --yes (pre-approved). Otherwise it warns and prompts
// interactively, and refuses non-interactively so scripts never trigger a silent restart.
//
//nolint:revive // assumeYes reflects the user's --yes flag, not internal control coupling.
func slcConfirmFunc(cmd *cobra.Command, assumeYes bool, action string) deploy.ConfirmFunc {
	if assumeYes {
		return nil
	}

	return func() (bool, error) {
		if !util.IsInteractiveStdin() {
			return false, errors.New(
				"this restarts the database; re-run with --yes to confirm, " +
					"or --no-restart to apply on the next start",
			)
		}

		_, _ = fmt.Fprintf(
			cmd.OutOrStdout(),
			"%s will restart the database. Open connections will be dropped and running "+
				"statements aborted.\nContinue? [y/N]: ",
			action,
		)

		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
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
			return renderSLCListJSON(cmd.OutOrStdout(), statuses)
		}

		renderSLCListText(statuses)

		return nil
	},
}

type slcListJSONItem struct {
	Language  string   `json:"language"`
	Flavor    string   `json:"flavor"`
	Version   string   `json:"version"`
	Aliases   []string `json:"aliases"`
	Installed bool     `json:"installed"`
}

func renderSLCListJSON(writer io.Writer, statuses []deploy.SLCStatus) error {
	items := make([]slcListJSONItem, 0, len(statuses))
	for _, status := range statuses {
		items = append(items, slcListJSONItem{
			Language:  status.Language,
			Flavor:    status.Flavor,
			Version:   status.Version,
			Aliases:   status.Aliases,
			Installed: status.Installed,
		})
	}

	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")

	return encoder.Encode(items)
}

func renderSLCListText(statuses []deploy.SLCStatus) {
	if len(statuses) == 0 {
		safePrint("No script language containers are available for this platform.")

		return
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

	safePrint(strings.TrimRight(builder.String(), "\n"))
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
	case deploy.SLCApplyRestarted:
		message += " Database restarted; the language is ready to use."
	case deploy.SLCApplyStarted:
		message += " Database started; the language is ready to use."
	case deploy.SLCApplyDeferred:
		message += " It will become available on the next start."
	default:
		// No suffix for an unrecognized outcome.
	}

	safePrint(message)
}

func printSLCUpdateResult(result *deploy.SLCUpdateResult) {
	message := fmt.Sprintf("Updated %s.", result.Entry.Flavor)
	if result.FromFlavor != "" && result.FromFlavor != result.Entry.Flavor {
		message = fmt.Sprintf("Updated %s to %s.", result.FromFlavor, result.Entry.Flavor)
	}

	switch result.Outcome {
	case deploy.SLCApplyRestarted:
		message += " Database restarted; the new version is ready to use."
	case deploy.SLCApplyStarted:
		message += " Database started; the new version is ready to use."
	case deploy.SLCApplyDeferred:
		message += " It will take effect on the next start."
	default:
		// No suffix for an unrecognized outcome.
	}

	safePrint(message)
}

func printSLCRemoveResult(result *deploy.SLCRemoveResult) {
	message := fmt.Sprintf("Removed %s.", result.Entry.Flavor)

	switch result.Outcome {
	case deploy.SLCApplyRestarted:
		message += " Database restarted; the language is no longer available."
	case deploy.SLCApplyStarted:
		message += " Database started."
	case deploy.SLCApplyDeferred:
		message += " It will no longer be loaded on the next start."
	default:
		// No suffix for an unrecognized outcome.
	}

	safePrint(message)
}

// nolint: gochecknoinits
func init() {
	requireDefaultDeploymentCompatibility(slcInstallCmd)
	requireInitializedDeploymentDir(slcInstallCmd)
	registerDeploymentDirFlag(slcInstallCmd, commonFlags)
	registerVerboseFlag(slcInstallCmd, commonFlags)
	slcInstallCmd.Flags().BoolVarP(&slcInstallOpts.Yes, "yes", "y", false,
		"Do not prompt for confirmation before restarting the database")
	slcInstallCmd.Flags().BoolVar(&slcInstallOpts.NoRestart, "no-restart", false,
		"Record the SLC without restarting; it activates on the next start")

	requireDefaultDeploymentCompatibility(slcUpdateCmd)
	requireInitializedDeploymentDir(slcUpdateCmd)
	registerDeploymentDirFlag(slcUpdateCmd, commonFlags)
	registerVerboseFlag(slcUpdateCmd, commonFlags)
	slcUpdateCmd.Flags().BoolVarP(&slcUpdateOpts.Yes, "yes", "y", false,
		"Do not prompt for confirmation before restarting the database")
	slcUpdateCmd.Flags().BoolVar(&slcUpdateOpts.NoRestart, "no-restart", false,
		"Record the update without restarting; it applies on the next start")

	requireDefaultDeploymentCompatibility(slcRemoveCmd)
	requireInitializedDeploymentDir(slcRemoveCmd)
	registerDeploymentDirFlag(slcRemoveCmd, commonFlags)
	registerVerboseFlag(slcRemoveCmd, commonFlags)
	slcRemoveCmd.Flags().BoolVarP(&slcRemoveOpts.Yes, "yes", "y", false,
		"Do not prompt for confirmation before restarting the database")
	slcRemoveCmd.Flags().BoolVar(&slcRemoveOpts.NoRestart, "no-restart", false,
		"Record the removal without restarting; it applies on the next start")

	registerDeploymentDirFlag(slcListCmd, commonFlags)
	registerOutputFlags(slcListCmd, commonFlags)

	slcCmd.AddCommand(slcInstallCmd)
	slcCmd.AddCommand(slcListCmd)
	slcCmd.AddCommand(slcUpdateCmd)
	slcCmd.AddCommand(slcRemoveCmd)
	rootCmd.AddCommand(slcCmd)
}
