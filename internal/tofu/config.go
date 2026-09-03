// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package tofu

import (
	"context"
	"errors"
	"path"
	"strings"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/presets"
	"github.com/exasol/exasol-personal/internal/resource"
)

// Default tofu configuration values.
const (
	DefaultVariablesFile = "variables_public.tf"
	DefaultVarsOutput    = "vars.tfvars"
	DefaultPlanFile      = "plan.tfplan"
	DefaultStateFile     = "terraform.tfstate"
)

// Config captures optional tofu settings from an infrastructure preset.
type Config struct {
	// All absolute!
	workDir        string
	tofuBinaryPath string
	variablesFile  string
	varsOutputFile string
	planeFile      string
	stateFile      string
	resolver       *resource.Resolver
}

// NewTofuConfigFromDeployment constructs a Tofu config from a deployment and preset.
func NewTofuConfigFromDeployment(
	deploymentDir string,
	presetTofuConfig presets.InfrastructureTofu,
	resolver *resource.Resolver,
) *Config {
	infraDir := path.Join(deploymentDir, config.InfrastructureFilesDirectory)

	return newTofuConfig(
		infraDir,
		presetTofuConfig.VariablesFile,
		presetTofuConfig.VarsOutputFile,
		resolver,
	)
}

// NewTofuConfigFromPreset constructs a Tofu config from a preset directory.
func NewTofuConfigFromPreset(
	infraDir string,
	presetTofuConfig presets.InfrastructureTofu,
) *Config {
	return newTofuConfig(
		infraDir,
		presetTofuConfig.VariablesFile,
		presetTofuConfig.VarsOutputFile,
		nil,
	)
}

// Construct a full tofu config.
// SSOT for all relative paths etc. Don't construct them anywhere else
// Pathes are either relative to work dir or absolute!
func newTofuConfig(
	workDir string,
	variablesRelFilepath string,
	varsOutputRelFilepath string,
	resolver *resource.Resolver,
) *Config {
	planFile := path.Join(workDir, DefaultPlanFile)
	stateFile := path.Join(workDir, DefaultStateFile)

	var variablesFile string
	if variablesRelFilepath == "" {
		variablesFile = DefaultVariablesFile
	} else {
		variablesFile = strings.TrimSpace(variablesRelFilepath)
	}
	variablesFile = path.Join(workDir, variablesFile)

	var varsOutputFile string
	if strings.TrimSpace(varsOutputRelFilepath) == "" {
		varsOutputFile = DefaultVarsOutput
	} else {
		varsOutputFile = strings.TrimSpace(varsOutputRelFilepath)
	}
	varsOutputFile = path.Join(workDir, varsOutputFile)

	return &Config{
		workDir:        workDir,
		variablesFile:  variablesFile,
		varsOutputFile: varsOutputFile,
		planeFile:      planFile,
		stateFile:      stateFile,
		resolver:       resolver,
	}
}

// WorkDir returns the directory used as Tofu's working directory.
func (c *Config) WorkDir() string {
	return c.workDir
}

// TofuBinaryPath returns the configured or cached Tofu executable path.
func (c *Config) TofuBinaryPath(ctx context.Context) (string, error) {
	if c.tofuBinaryPath != "" {
		return c.tofuBinaryPath, nil
	}

	if c.resolver == nil {
		return "", errors.New("tofu binary path is not configured")
	}

	binaryPath, err := c.resolver.Resolve(ctx, "tofu")
	if err != nil {
		return "", err
	}

	c.tofuBinaryPath = binaryPath

	return c.tofuBinaryPath, nil
}

// VariablesFile returns the variables file path.
func (c *Config) VariablesFile() string {
	return c.variablesFile
}

// VarsOutputFile returns the variables output file path.
func (c *Config) VarsOutputFile() string {
	return c.varsOutputFile
}

// PlanFile returns the plan file path.
func (c *Config) PlanFile() string {
	return c.planeFile
}

// StateFile returns the state file path.
func (c *Config) StateFile() string {
	return c.stateFile
}
