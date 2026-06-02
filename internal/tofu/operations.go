// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package tofu

import (
	"context"
	"io"
	"log/slog"
	"os"
)

// Public operations of the tofu package

// Prepare a deployment directory for tofu usage.
//
// Writes the tfvars file (merging defaults from the infrastructure preset and
// user overrides).
//
// If tofu is not configured (cfg is nil or empty), this function is a no-op.
func Prepare(
	cfg *Config,
	infraVars map[string]string,
) error {
	slog.Info("tofu: prepare workspace")
	if err := Configure(cfg, infraVars); err != nil {
		return err
	}

	return SetupWorkspace(cfg)
}

func Configure(
	cfg *Config,
	infraVars map[string]string,
) error {
	slog.Info("tofu: configure workspace")

	slog.Debug("tofu: reading variables file", "path", cfg.VariablesFile())
	variableData, err := os.ReadFile(cfg.VariablesFile())
	if err != nil {
		return err
	}

	slog.Debug("tofu: parsing variables file content", "size", len(variableData))
	defaults, err := ParseVarFile(variableData, cfg.VariablesFile())
	if err != nil {
		return err
	}

	slog.Debug(
		"tofu: parsing variables file content",
		"defaults",
		len(defaults),
		"overrides",
		len(infraVars),
	)
	overrides, err := ParseOverrideStrings(defaults, infraVars)
	if err != nil {
		return err
	}

	slog.Info("tofu: writing TF vars", "path", cfg.VarsOutputFile())

	return writeVarsFileWithOverrides(
		cfg.VarsOutputFile(),
		defaults,
		overrides,
	)
}

func SetupWorkspace(_ *Config) error {
	return nil
}

// Initialize tofu for a deployment.
func Initialize(
	ctx context.Context,
	cfg *Config,
	out, outErr io.Writer,
	lockfileMode LockfileMode,
) error {
	slog.Info("tofu: initialize workspace")

	tofuRunner, err := NewTofuRunner(ctx, cfg, out, outErr)
	if err != nil {
		return err
	}

	return tofuRunner.Init(ctx, lockfileMode)
}

// Plan for a tofu deployment.
func Plan(
	ctx context.Context,
	cfg *Config,
	out, outErr io.Writer,
) error {
	slog.Info("tofu: plan infrastructure")

	tofuRunner, err := NewTofuRunner(ctx, cfg, out, outErr)
	if err != nil {
		return err
	}

	return tofuRunner.Plan(ctx, cfg.PlanFile(), cfg.VarsOutputFile(), cfg.StateFile())
}

// Apply a tofu action with variable overrides (used for start/stop).
func ApplyPlan(
	ctx context.Context,
	cfg *Config,
	out, outErr io.Writer,
) error {
	slog.Info("tofu: deploy infrastructure")

	tofuRunner, err := NewTofuRunner(ctx, cfg, out, outErr)
	if err != nil {
		return err
	}

	applyOpts := ApplyOptions{
		// Must use plan file
		PlanFilePath: cfg.PlanFile(),
		// Must not use vars file
		StateFilePath: cfg.StateFile(),
	}

	return tofuRunner.Apply(ctx, applyOpts)
}

// Apply a tofu action with variable overrides (used for start/stop).
func ApplyAction(
	ctx context.Context,
	cfg *Config,
	action string,
	out, outErr io.Writer,
) error {
	slog.Info("tofu: change infrastructure")

	tofuRunner, err := NewTofuRunner(ctx, cfg, out, outErr)
	if err != nil {
		return err
	}

	applyOpts := ApplyOptions{
		// Cannot use plan file
		VarArgs: []string{action},
		// Must use Vars file
		VarsFilePath:  cfg.VarsOutputFile(),
		StateFilePath: cfg.StateFile(),
	}

	return tofuRunner.Apply(ctx, applyOpts)
}

// Destroy releases tofu-managed resources for a deployment.
func Destroy(
	ctx context.Context,
	cfg *Config,
	out, outErr io.Writer,
) error {
	slog.Info("tofu: destroy infrastructure")

	tofuRunner, err := NewTofuRunner(ctx, cfg, out, outErr)
	if err != nil {
		return err
	}

	return tofuRunner.Destroy(ctx, cfg.VarsOutputFile(), cfg.StateFile())
}
