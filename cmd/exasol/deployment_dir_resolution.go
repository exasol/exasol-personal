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
	legacyWorkflowStateMarker = ".workflowState.json"
)

type deploymentDirSource int

const (
	deploymentDirSourceNone deploymentDirSource = iota
	deploymentDirSourceExplicit
	deploymentDirSourceCurrent
	deploymentDirSourceDefault
)

func resolveDeploymentDirForCommand(cmd *cobra.Command, state *CommonFlags) error {
	deployment, source, err := resolveDeploymentDir(cmd, state)
	if err != nil {
		return err
	}
	if source == deploymentDirSourceNone {
		return nil
	}

	state.DeploymentDir = deployment.Root()
	slog.Info(
		"using deployment directory",
		"path", deployment.Root(),
		"source", source.String(),
	)

	return nil
}

func (source deploymentDirSource) String() string {
	switch source {
	case deploymentDirSourceExplicit:
		return "explicit"
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
	flag := deploymentDirFlag(cmd)
	if flag == nil {
		return state.Deployment(), deploymentDirSourceNone, nil
	}
	if flag.Changed {
		return state.Deployment(), deploymentDirSourceExplicit, nil
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

func defaultDeploymentDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory for default deployment directory: %w", err)
	}

	return filepath.Join(home, ".exasol", "personal", "deployments", "default"), nil
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
