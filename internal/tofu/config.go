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
	"github.com/exasol/exasol-personal/internal/runtimeartifacts"
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
	manager        *runtimeartifacts.Manager
}

// Construct a tofu config from a deployment directory and a preset manifest.
func NewTofuConfigFromDeployment(
	deploymentDir string,
	presetTofuConfig presets.InfrastructureTofu,
	manager *runtimeartifacts.Manager,
) *Config {
	infraDir := path.Join(deploymentDir, config.InfrastructureFilesDirectory)

	return newTofuConfig(
		infraDir,
		presetTofuConfig.VariablesFile,
		presetTofuConfig.VarsOutputFile,
		manager,
	)
}

// Construct a tofu config from a infra directory and a preset manifest.
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
	manager *runtimeartifacts.Manager,
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
		manager:        manager,
	}
}

// The directory that all file paths are relative to if they aren't absolute.
// And that should be used as the working dir for tofu.
func (c *Config) WorkDir() string {
	return c.workDir
}

// Relative to the deployment root or absolute.
func (c *Config) TofuBinaryPath(ctx context.Context) (string, error) {
	if c.tofuBinaryPath != "" {
		return c.tofuBinaryPath, nil
	}

	if c.manager == nil {
		return "", errors.New("tofu binary path is not configured")
	}

	binaryPath, err := c.manager.Request(ctx, "tofu")
	if err != nil {
		return "", err
	}

	c.tofuBinaryPath = binaryPath

	return c.tofuBinaryPath, nil
}

// Relative to the work dir or absolute.
func (c *Config) VariablesFile() string {
	return c.variablesFile
}

// Relative to the work dir or absolute.
func (c *Config) VarsOutputFile() string {
	return c.varsOutputFile
}

// Relative to the work dir or absolute.
func (c *Config) PlanFile() string {
	return c.planeFile
}

// Relative to the work dir or absolute.
func (c *Config) StateFile() string {
	return c.stateFile
}
