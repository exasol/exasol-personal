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

var slcCustomInstallOpts = struct {
	AutoApprove bool
	File        string
	URL         string
	Alias       string
	Language    string
}{}

var slcCustomUpdateOpts = struct {
	File     string
	URL      string
	Alias    string
	Language string
}{}

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
	Long: "Install an official script language container by alias.\n\n" +
		"For a user-supplied container, use `exasol slc custom install`.",
	Args: cobra.ExactArgs(1),
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
	Short: "Update an installed official script language container",
	Long: "Update an installed official script language container by alias.\n\n" +
		"For a user-supplied container, use `exasol slc custom update`.",
	Args: cobra.ExactArgs(1),
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
			addTerminalNotice(fmt.Sprintf("Run `exasol slc install %s` to install it.", alias))

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

		isCustom, err := deploy.IsCustomSLCAlias(commonFlags.Deployment(), alias)
		if err != nil {
			return err
		}
		if isCustom {
			return fmt.Errorf(
				"%q is a custom SLC; remove it with `exasol slc custom remove %s`", alias, alias,
			)
		}

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

var slcCustomCmd = &cobra.Command{
	Use:   "custom",
	Short: "Manage user-supplied (custom) script language containers",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return cmd.Help()
	},
}

var slcCustomInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install a custom script language container",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true

		return runSLCCustomInstall(cmd)
	},
}

var slcCustomUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Replace an installed custom script language container",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true

		return runSLCCustomUpdate(cmd)
	},
}

var slcCustomRemoveCmd = &cobra.Command{
	Use:   "remove <alias>",
	Short: "Remove an installed custom script language container",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		return runSLCCustomRemove(cmd, args[0])
	},
}

func runSLCCustomInstall(cmd *cobra.Command) error {
	result, err := deploy.InstallCustomSLC(
		cmd.Context(),
		commonFlags.Deployment(),
		deploy.CustomSLCInstallOpts{
			Alias:    slcCustomInstallOpts.Alias,
			Language: slcCustomInstallOpts.Language,
			File:     slcCustomInstallOpts.File,
			URL:      slcCustomInstallOpts.URL,
		},
		customSLCConfirmFunc(cmd, slcCustomInstallOpts.AutoApprove),
	)
	if errors.Is(err, deploy.ErrSLCOperationCancelled) {
		addTerminalNotice(slcOperationAbortedMessage)

		return nil
	}
	if err != nil {
		return err
	}

	if result.AlreadyInstalled {
		addTerminalOutput(
			result.Alias + " is already installed with this container. Nothing to do.",
		)

		return nil
	}

	verb := "Installed"
	if result.Replaced {
		verb = "Replaced"
	}
	addTerminalOutput(fmt.Sprintf(
		"%s custom SLC %q (language: %s). It is active for new sessions.",
		verb, result.Alias, result.Language,
	))

	return nil
}

func runSLCCustomUpdate(cmd *cobra.Command) error {
	result, err := deploy.UpdateCustomSLC(
		cmd.Context(),
		commonFlags.Deployment(),
		deploy.CustomSLCInstallOpts{
			Alias:    slcCustomUpdateOpts.Alias,
			Language: slcCustomUpdateOpts.Language,
			File:     slcCustomUpdateOpts.File,
			URL:      slcCustomUpdateOpts.URL,
		},
	)
	if errors.Is(err, deploy.ErrSLCOperationCancelled) {
		addTerminalNotice(slcOperationAbortedMessage)

		return nil
	}
	if err != nil {
		return err
	}

	if !result.Found {
		addTerminalOutput(fmt.Sprintf(
			"No custom SLC matching %q is installed, so there is nothing to update.", result.Alias,
		))

		return nil
	}
	if result.Unchanged {
		addTerminalOutput(fmt.Sprintf(
			"Custom SLC %q is already up to date. Nothing to do.", result.Alias,
		))

		return nil
	}

	addTerminalOutput(fmt.Sprintf(
		"Updated custom SLC %q. It is active for new sessions.", result.Alias,
	))

	return nil
}

func runSLCCustomRemove(cmd *cobra.Command, alias string) error {
	result, err := deploy.RemoveCustomSLC(cmd.Context(), commonFlags.Deployment(), alias)
	if err != nil {
		return err
	}
	if !result.Found {
		addTerminalOutput(fmt.Sprintf(
			"No SLC matching %q is installed, so there is nothing to remove.", alias,
		))

		return nil
	}

	addTerminalOutput(fmt.Sprintf(
		"Removed custom SLC %q. It is no longer available to new sessions.", result.Alias,
	))

	return nil
}

// customSLCConfirmFunc returns nil (pre-approved) when --auto-approve is set; otherwise it
// refuses non-interactively so scripts never override an SLC silently.
//
//nolint:revive // autoApprove reflects the user's flag, not internal control coupling.
func customSLCConfirmFunc(cmd *cobra.Command, autoApprove bool) deploy.CustomSLCConfirm {
	if autoApprove {
		return nil
	}

	return func(prompt string) (bool, error) {
		if !util.IsInteractiveStdin() {
			return false, errors.New(prompt + "; re-run with --auto-approve to proceed")
		}

		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s.\nContinue? [y/N]: ", prompt)

		line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return false, err
		}

		return confirmYes(line), nil
	}
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
		customs, err := deploy.CustomSLCStatuses(commonFlags.Deployment())
		if err != nil {
			return err
		}

		if commonFlags.OutputJson {
			return queueSLCListJSON(statuses, customs)
		}

		renderSLCListText(statuses, customs)

		return nil
	},
}

type slcListJSONItem struct {
	Type      string   `json:"type"`
	Language  string   `json:"language"`
	Flavor    string   `json:"flavor,omitempty"`
	Version   string   `json:"version,omitempty"`
	Aliases   []string `json:"aliases,omitempty"`
	Alias     string   `json:"alias,omitempty"`
	Source    string   `json:"source,omitempty"`
	Installed bool     `json:"installed"`
}

func renderSLCListJSON(
	writer io.Writer,
	statuses []deploy.SLCStatus,
	customs []deploy.CustomSLCStatus,
) error {
	content, err := formatSLCListJSON(statuses, customs)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(writer, content)

	return err
}

func queueSLCListJSON(statuses []deploy.SLCStatus, customs []deploy.CustomSLCStatus) error {
	content, err := formatSLCListJSON(statuses, customs)
	if err != nil {
		return err
	}
	addTerminalOutput(content)

	return nil
}

func formatSLCListJSON(
	statuses []deploy.SLCStatus,
	customs []deploy.CustomSLCStatus,
) (string, error) {
	items := make([]slcListJSONItem, 0, len(statuses)+len(customs))
	for _, status := range statuses {
		items = append(items, slcListJSONItem{
			Type:      "official",
			Language:  status.Language,
			Flavor:    status.Flavor,
			Version:   status.Version,
			Aliases:   status.Aliases,
			Installed: status.Installed,
		})
	}
	for _, custom := range customs {
		items = append(items, slcListJSONItem{
			Type:      "custom",
			Language:  custom.Language,
			Alias:     custom.Alias,
			Source:    custom.Source,
			Installed: true,
		})
	}

	content, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return "", err
	}

	return string(content), nil
}

func renderSLCListText(statuses []deploy.SLCStatus, customs []deploy.CustomSLCStatus) {
	addTerminalOutput(formatSLCListText(statuses, customs))
}

func formatSLCListText(statuses []deploy.SLCStatus, customs []deploy.CustomSLCStatus) string {
	if len(statuses) == 0 && len(customs) == 0 {
		return slcNoneAvailableText
	}

	var builder strings.Builder
	const columnPadding = 2
	writer := tabwriter.NewWriter(&builder, 0, 0, columnPadding, ' ', 0)
	if len(statuses) > 0 {
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
	}
	if len(customs) > 0 {
		if len(statuses) > 0 {
			_, _ = fmt.Fprintln(writer)
		}
		_, _ = fmt.Fprintln(writer, "CUSTOM ALIAS\tLANGUAGE\tSOURCE")
		for _, custom := range customs {
			_, _ = fmt.Fprintf(writer, "%s\t%s\t%s\n", custom.Alias, custom.Language, custom.Source)
		}
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

	requireDefaultDeploymentCompatibility(slcCustomInstallCmd)
	requireInitializedDeploymentDir(slcCustomInstallCmd)
	registerDeploymentDirFlag(slcCustomInstallCmd, commonFlags)
	slcCustomInstallCmd.Flags().BoolVar(&slcCustomInstallOpts.AutoApprove, "auto-approve", false,
		"Do not prompt before overriding a built-in alias or replacing a custom SLC")
	slcCustomInstallCmd.Flags().StringVar(&slcCustomInstallOpts.File, "file", "",
		"Install the custom SLC from a local container tarball")
	slcCustomInstallCmd.Flags().StringVar(&slcCustomInstallOpts.URL, "url", "",
		"Install the custom SLC downloaded from an HTTPS URL")
	slcCustomInstallCmd.Flags().StringVar(&slcCustomInstallOpts.Alias, "alias", "",
		"Alias for the custom SLC (used in CREATE <alias> SCALAR SCRIPT)")
	slcCustomInstallCmd.Flags().StringVar(&slcCustomInstallOpts.Language, "language", "",
		"Language the custom SLC provides: python, java, or r")

	requireDefaultDeploymentCompatibility(slcCustomUpdateCmd)
	requireInitializedDeploymentDir(slcCustomUpdateCmd)
	registerDeploymentDirFlag(slcCustomUpdateCmd, commonFlags)
	slcCustomUpdateCmd.Flags().StringVar(&slcCustomUpdateOpts.File, "file", "",
		"Update the custom SLC from a local container tarball")
	slcCustomUpdateCmd.Flags().StringVar(&slcCustomUpdateOpts.URL, "url", "",
		"Update the custom SLC downloaded from an HTTPS URL")
	slcCustomUpdateCmd.Flags().StringVar(&slcCustomUpdateOpts.Alias, "alias", "",
		"Alias of the custom SLC to update")
	slcCustomUpdateCmd.Flags().StringVar(&slcCustomUpdateOpts.Language, "language", "",
		"Override the language the custom SLC provides: python, java, or r")

	requireDefaultDeploymentCompatibility(slcCustomRemoveCmd)
	requireInitializedDeploymentDir(slcCustomRemoveCmd)
	registerDeploymentDirFlag(slcCustomRemoveCmd, commonFlags)

	slcCustomCmd.AddCommand(slcCustomInstallCmd)
	slcCustomCmd.AddCommand(slcCustomUpdateCmd)
	slcCustomCmd.AddCommand(slcCustomRemoveCmd)

	slcCmd.AddCommand(slcInstallCmd)
	slcCmd.AddCommand(slcListCmd)
	slcCmd.AddCommand(slcUpdateCmd)
	slcCmd.AddCommand(slcRemoveCmd)
	slcCmd.AddCommand(slcCustomCmd)
	rootCmd.AddCommand(slcCmd)
}
