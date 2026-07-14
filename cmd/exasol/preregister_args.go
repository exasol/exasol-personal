// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"fmt"
	"io"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const helpCommandName = "help"

func preregisteredCommand(args []string) (*cobra.Command, []string) {
	cmd, remainingArgs, err := rootCmd.Find(args)
	if err != nil {
		return nil, nil
	}

	return cmd, remainingArgs
}

func preregisteredCommandIs(args []string, expected *cobra.Command) bool {
	cmd, _ := preregisteredCommand(args)

	return cmd == expected
}

func preregisteredPositionals(args []string) ([]string, error) {
	flagset := pflag.NewFlagSet("preregister-args", pflag.ContinueOnError)
	flagset.SetOutput(io.Discard)
	flagset.SetInterspersed(true)
	flagset.ParseErrorsAllowlist.UnknownFlags = true
	flagset.BoolP(helpCommandName, "h", false, "")

	if err := flagset.Parse(args); err != nil && !errors.Is(err, pflag.ErrHelp) {
		return nil, fmt.Errorf("cannot parse pre-registration args: %w", err)
	}

	return flagset.Args(), nil
}

func deploymentDirFromRawArgs(args []string) (config.DeploymentDir, error) {
	flagset := pflag.NewFlagSet("deployment-dir-pre-scan", pflag.ContinueOnError)
	flagset.SetOutput(io.Discard)
	flagset.SetInterspersed(true)
	flagset.ParseErrorsAllowlist.UnknownFlags = true

	var deploymentDir string
	var name string
	flagset.StringVar(&deploymentDir, deploymentDirFlagName, "", "")
	flagset.StringVarP(&name, deploymentNameFlagName, "d", "", "")

	if err := flagset.Parse(args); err != nil && !errors.Is(err, pflag.ErrHelp) {
		return config.DeploymentDir{}, fmt.Errorf("cannot parse deployment directory: %w", err)
	}

	deployment, _, err := resolveDeploymentDirFromValues(deploymentDirFlagValues{
		deploymentDir:        deploymentDir,
		deploymentDirChanged: flagset.Changed(deploymentDirFlagName),
		name:                 name,
		nameChanged:          flagset.Changed(deploymentNameFlagName),
	})

	return deployment, err
}
