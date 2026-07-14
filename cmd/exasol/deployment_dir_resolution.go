// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const (
	deploymentDirFlagName     = "deployment-dir"
	deploymentNameFlagName    = "deployment"
	legacyWorkflowStateMarker = ".workflowState.json"
)

type deploymentDirSource int

const (
	deploymentDirSourceNone deploymentDirSource = iota
	deploymentDirSourceExplicit
	deploymentDirSourceNamed
	deploymentDirSourceCurrent
	deploymentDirSourceDefault
)

// deploymentDirFlagValues carries the already-read state of the
// --deployment-dir and --deployment/-d flags into the shared precedence resolver,
// independent of whether they were read from a parsed *cobra.Command or a
// pre-Cobra raw-args pflag.FlagSet.
type deploymentDirFlagValues struct {
	deploymentDir        string
	deploymentDirChanged bool
	name                 string
	nameChanged          bool
}

func resolveDeploymentDirForCommand(cmd *cobra.Command, state *CommonFlags) error {
	deployment, source, err := resolveDeploymentDir(cmd, state)
	if err != nil {
		return err
	}
	if source == deploymentDirSourceNone {
		return nil
	}

	state.DeploymentDir = deployment.Root()
	slog.Debug(
		"using deployment directory",
		"path", deployment.Root(),
		"source", source.String(),
	)
	addResolvedDeploymentDirNotice(deployment, source, state.DeploymentName)

	return nil
}

// addResolvedDeploymentDirNotice makes an implicit deployment-directory
// selection visible to the user. Explicit (--deployment-dir) and current
// (cwd auto-detected) selections are already visible to the user without a
// notice: they either typed the flag or are sitting in the directory.
// Notices go through addTerminalNotice, which is stderr-only, so JSON stdout
// output is never affected.
func addResolvedDeploymentDirNotice(
	deployment config.DeploymentDir,
	source deploymentDirSource,
	name string,
) {
	switch source {
	case deploymentDirSourceDefault:
		addTerminalNotice("Using default deployment directory: " + deployment.Root())
	case deploymentDirSourceNamed:
		addTerminalNotice(fmt.Sprintf(
			"Using named deployment directory %q: %s", name, deployment.Root(),
		))
	case deploymentDirSourceNone, deploymentDirSourceExplicit, deploymentDirSourceCurrent:
	default:
	}
}

func (source deploymentDirSource) String() string {
	switch source {
	case deploymentDirSourceExplicit:
		return "explicit"
	case deploymentDirSourceNamed:
		return "named"
	case deploymentDirSourceCurrent:
		return "current"
	case deploymentDirSourceDefault:
		return "default"
	case deploymentDirSourceNone:
		fallthrough
	default:
		return "none"
	}
}

func resolveDeploymentDir(
	cmd *cobra.Command,
	state *CommonFlags,
) (config.DeploymentDir, deploymentDirSource, error) {
	dirFlag := deploymentDirFlag(cmd)
	nameFlag := deploymentNameFlag(cmd)
	if dirFlag == nil && nameFlag == nil {
		return state.Deployment(), deploymentDirSourceNone, nil
	}

	return resolveDeploymentDirFromValues(deploymentDirFlagValues{
		deploymentDir:        state.DeploymentDir,
		deploymentDirChanged: dirFlag != nil && dirFlag.Changed,
		name:                 state.DeploymentName,
		nameChanged:          nameFlag != nil && nameFlag.Changed,
	})
}

// resolveDeploymentDirFromValues is the single precedence implementation
// shared by resolveDeploymentDir (parsed *cobra.Command, real command
// execution) and deploymentDirFromRawArgs (pre-Cobra raw-args pre-scan).
//
// It does not itself reject values.deploymentDirChanged &&
// values.nameChanged: for real command execution, Cobra's
// MarkFlagsMutuallyExclusive combined with an early ValidateFlagGroups call
// in root's PersistentPreRunE already rejects that combination before this
// function ever runs. For the raw-args pre-scan (used only to decide which
// dynamic flags to register), the explicit --deployment-dir value simply
// takes precedence, which is harmless since the real Cobra parse still
// rejects the command afterward.
func resolveDeploymentDirFromValues(
	values deploymentDirFlagValues,
) (config.DeploymentDir, deploymentDirSource, error) {
	if values.deploymentDirChanged {
		return config.NewDeploymentDir(values.deploymentDir), deploymentDirSourceExplicit, nil
	}
	if values.nameChanged {
		namedDir, err := config.NamedDeploymentDirPath(values.name)
		if err != nil {
			return config.DeploymentDir{}, deploymentDirSourceNone, err
		}

		return config.NewDeploymentDir(namedDir), deploymentDirSourceNamed, nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return config.DeploymentDir{}, deploymentDirSourceNone, fmt.Errorf(
			"get current directory: %w",
			err,
		)
	}
	if recognized, err := isRecognizedDeploymentDir(cwd); err != nil {
		return config.DeploymentDir{}, deploymentDirSourceNone, err
	} else if recognized {
		return config.NewDeploymentDir(cwd), deploymentDirSourceCurrent, nil
	}

	defaultDir, err := defaultDeploymentDir()
	if err != nil {
		return config.DeploymentDir{}, deploymentDirSourceNone, err
	}

	return config.NewDeploymentDir(defaultDir), deploymentDirSourceDefault, nil
}

func deploymentDirFlag(cmd *cobra.Command) *pflag.Flag {
	if flag := cmd.Flags().Lookup(deploymentDirFlagName); flag != nil {
		return flag
	}

	return cmd.InheritedFlags().Lookup(deploymentDirFlagName)
}

func deploymentNameFlag(cmd *cobra.Command) *pflag.Flag {
	if flag := cmd.Flags().Lookup(deploymentNameFlagName); flag != nil {
		return flag
	}

	return cmd.InheritedFlags().Lookup(deploymentNameFlagName)
}

func defaultDeploymentDir() (string, error) {
	return config.DefaultDeploymentDirPath()
}

// defaultDeploymentDirDisplayPath and deploymentsRootDisplayPath resolve the
// launcher-managed deployment paths using the current platform's real path
// conventions, for use in --help text generated at startup. They fall back
// to a platform-neutral description in the rare case the home directory
// cannot be resolved (e.g. HOME/USERPROFILE unset), rather than failing
// --help entirely.
func defaultDeploymentDirDisplayPath() string {
	path, err := defaultDeploymentDir()
	if err != nil {
		return "the launcher's default deployment directory in your home directory"
	}

	return path
}

func deploymentsRootDisplayPath() string {
	root, err := config.DeploymentsRootPath()
	if err != nil {
		return "the launcher's deployment directories folder in your home directory"
	}

	return root
}

func isRecognizedDeploymentDir(path string) (bool, error) {
	for _, marker := range []string{
		config.ExasolPersonalStateFileName,
		config.DeploymentVersionMarkerFileName,
		legacyWorkflowStateMarker,
	} {
		exists, err := pathExists(filepath.Join(path, marker))
		if err != nil {
			return false, err
		}
		if exists {
			return true, nil
		}
	}

	return false, nil
}

func pathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}

	return false, err
}
