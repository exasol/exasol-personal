// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package tofu

import (
	"path"
	"strings"

	"github.com/exasol/exasol-personal/assets/tofubin"
	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/presets"
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
}

// Construct a tofu config from a deployment directory and a preset manifest.
func NewTofuConfigFromDeployment(
	deploymentDir string,
	presetTofuConfig presets.InfrastructureTofu,
) *Config {
	infraDir := path.Join(deploymentDir, config.InfrastructureFilesDirectory)

	return NewTofuConfigFromPreset(infraDir, presetTofuConfig)
}

// Construct a tofu config from a infra directory and a preset manifest.
func NewTofuConfigFromPreset(infraDir string, presetTofuConfig presets.InfrastructureTofu) *Config {
	return newTofuConfig(infraDir, presetTofuConfig.VariablesFile, presetTofuConfig.VarsOutputFile)
}

// Construct a full tofu config.
// SSOT for all relative paths etc. Don't construct them anywhere else
// Pathes are either relative to work dir or absolute!
func newTofuConfig(
	workDir string,
	variablesRelFilepath string,
	varsOutputRelFilepath string,
) *Config {
	planFile := path.Join(workDir, DefaultPlanFile)
	stateFile := path.Join(workDir, DefaultStateFile)
	tofuBinaryPath := path.Join(workDir, tofubin.TofuBinaryName)

	var variablesFile string
	if variablesRelFilepath == "" {
		variablesFile = DefaultVariablesFile
	} else {
		variablesFile = strings.TrimSpace(variablesRelFilepath)
	}
	variablesFile = path.Join(workDir, variablesFile)

	var varsOutputFile string
	if varsOutputFile == "" {
		varsOutputFile = DefaultVarsOutput
	} else {
		varsOutputFile = strings.TrimSpace(varsOutputRelFilepath)
	}
	varsOutputFile = path.Join(workDir, varsOutputFile)

	return &Config{workDir, tofuBinaryPath, variablesFile, varsOutputFile, planFile, stateFile}
}

// The directory that all file paths are relative to if they aren't absolute.
// And that should be used as the working dir for tofu.
func (c *Config) WorkDir() string {
	return c.workDir
}

// Relative to the work dir or absolute.
func (c *Config) TofuBinaryPath() string {
	return c.tofuBinaryPath
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
