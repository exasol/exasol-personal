// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/spf13/cobra"
)

const deploymentsCmdShortDesc = "Manage launcher-managed deployment directories"

// deploymentsCmdLongDesc is built at startup (not a const) because it
// interpolates the launcher's deployments root path using the current
// platform's real path conventions.
var deploymentsCmdLongDesc = deploymentsCmdShortDesc + fmt.Sprintf(`

Deployment directories are the launcher-managed directories under
%s: the default deployment directory and any
named deployment directory selected via --deployment/-d. This does not include
deployment directories selected via an arbitrary --deployment-dir path.
`, deploymentsRootDisplayPath())

const (
	deploymentStatusInitialized    = "initialized"
	deploymentStatusNotInitialized = "not_initialized"
)

// deploymentListEntry describes one launcher-managed deployment directory for
// `exasol deployments list`. Note: unlike `exasol status`, Status here is only
// ever "initialized" or "not_initialized" — see registerDeploymentsCommands
// doc comment for why this intentionally does not run full deploy.Status.
type deploymentListEntry struct {
	Name           string `json:"name"`
	Path           string `json:"path"`
	Status         string `json:"status"`
	Infrastructure string `json:"infrastructure,omitempty"`
	Installation   string `json:"installation,omitempty"`
	Active         bool   `json:"active"`
}

var deploymentsCmd = &cobra.Command{
	Use:   "deployments",
	Short: deploymentsCmdShortDesc,
	Long:  deploymentsCmdLongDesc,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return cmd.Help()
	},
}

var deploymentsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List launcher-managed deployment directories",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
		entries, err := listDeploymentDirectories()
		if err != nil {
			return err
		}
		if commonFlags.OutputJson {
			return addJSONTerminalOutput(entries)
		}

		return addRenderedTerminalOutput(func(writer io.Writer) error {
			return renderDeploymentsListText(writer, entries)
		})
	},
}

// listDeploymentDirectories scans config.DeploymentsRootPath for deployment
// directories. It intentionally does not accept --deployment-dir or
// --deployment/-d: deploymentsListCmd registers neither flag, so root's
// PersistentPreRunE never resolves a deployment directory (and never emits
// the resolved-directory notice) for this command; see
// registerDeploymentsCommands.
func listDeploymentDirectories() ([]deploymentListEntry, error) {
	root, err := config.DeploymentsRootPath()
	if err != nil {
		return nil, err
	}

	dirEntries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return []deploymentListEntry{}, nil
		}

		return nil, fmt.Errorf("read deployments directory %q: %w", root, err)
	}

	activePath, err := activeDeploymentDirPath()
	if err != nil {
		return nil, err
	}

	entries := make([]deploymentListEntry, 0, len(dirEntries))
	for _, dirEntry := range dirEntries {
		if !dirEntry.IsDir() {
			continue
		}

		path := filepath.Join(root, dirEntry.Name())
		entries = append(entries, deploymentListEntryFor(dirEntry.Name(), path, activePath))
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })

	return entries, nil
}

// deploymentListEntryFor never fails on its own: an unreadable or malformed
// entry is reported as not_initialized rather than aborting the whole
// listing. A permission error on the deployments root itself, by contrast,
// is surfaced by listDeploymentDirectories as a command failure.
func deploymentListEntryFor(name, path, activePath string) deploymentListEntry {
	entry := deploymentListEntry{
		Name:   name,
		Path:   path,
		Status: deploymentStatusNotInitialized,
		Active: path == activePath,
	}

	// Reuses the same recognition check as current-working-directory
	// detection elsewhere in the CLI (modern state file, deployment version
	// marker, legacy .workflowState.json), so this listing never disagrees
	// with the rest of the CLI about whether a directory is a real
	// deployment.
	recognized, err := isRecognizedDeploymentDir(path)
	if err != nil || !recognized {
		return entry
	}

	entry.Status = deploymentStatusInitialized

	identity, err := deploy.ResolveDeploymentPresetIdentity(config.NewDeploymentDir(path))
	if err != nil {
		return entry
	}

	entry.Infrastructure = identity.Infrastructure
	entry.Installation = identity.Installation

	return entry
}

// activeDeploymentDirPath reports what resolution would select right now with
// no --deployment-dir or --deployment/-d flag, from the current working
// directory. It calls the shared pure precedence function directly rather than
// resolveDeploymentDirForCommand, which also emits the resolved-directory
// visibility notice — inappropriate here since deploymentsListCmd has its own
// dedicated output.
func activeDeploymentDirPath() (string, error) {
	deployment, _, err := resolveDeploymentDirFromValues(deploymentDirFlagValues{})
	if err != nil {
		return "", err
	}

	return deployment.Root(), nil
}

func renderDeploymentsListText(writer io.Writer, entries []deploymentListEntry) error {
	if len(entries) == 0 {
		_, err := fmt.Fprintln(writer, "No deployment directories found.")

		return err
	}

	for _, entry := range entries {
		if _, err := fmt.Fprintln(writer, deploymentListEntryText(entry)); err != nil {
			return err
		}
	}

	return nil
}

func deploymentListEntryText(entry deploymentListEntry) string {
	marker := " "
	if entry.Active {
		marker = "*"
	}

	preset := ""
	if entry.Infrastructure != "" || entry.Installation != "" {
		preset = fmt.Sprintf(" preset=%s/%s", entry.Infrastructure, entry.Installation)
	}

	return fmt.Sprintf(
		"%s %s status=%s%s path=%s", marker, entry.Name, entry.Status, preset, entry.Path,
	)
}

func registerDeploymentsCommands() {
	// deploymentsListCmd deliberately does not register --deployment-dir or
	// --deployment/-d: it enumerates every deployment directory, so
	// per-invocation directory selection does not apply.
	deploymentsListCmd.Flags().SortFlags = false
	registerOutputFlags(deploymentsListCmd, commonFlags)

	deploymentsCmd.AddCommand(deploymentsListCmd)
}

// nolint: gochecknoinits
func init() {
	registerDeploymentsCommands()
	rootCmd.AddCommand(deploymentsCmd)
}
