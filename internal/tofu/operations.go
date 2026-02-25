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
// user overrides) and writes the embedded tofu binary into the deployment
// directory.
//
// If tofu is not configured (cfg is nil or empty), this function is a no-op.
func Prepare(
	cfg *Config,
	infraVars map[string]string,
) error {
	slog.Info("tofu: prepare workspace")

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
	if err := writeVarsFileWithOverrides(
		cfg.VarsOutputFile(),
		defaults,
		overrides,
	); err != nil {
		return err
	}

	slog.Info("tofu: saving tofu executable", "path", cfg.TofuBinaryPath())

	return WriteBinary(cfg.TofuBinaryPath())
}

// Initialize tofu for a deployment.
func Initialize(
	ctx context.Context,
	cfg Config,
	out, outErr io.Writer,
	lockfileMode LockfileMode,
) error {
	slog.Info("tofu: initialize workspace")

	tofuRunner := NewTofuRunner(cfg, out, outErr)

	return tofuRunner.Init(ctx, lockfileMode)
}

// Plan for a tofu deployment.
func Plan(
	ctx context.Context,
	cfg Config,
	out, outErr io.Writer,
) error {
	slog.Info("tofu: plan infrastructure")

	tofuRunner := NewTofuRunner(cfg, out, outErr)

	return tofuRunner.Plan(ctx, cfg.PlanFile(), cfg.VarsOutputFile(), cfg.StateFile())
}

// Apply a tofu action with variable overrides (used for start/stop).
func ApplyPlan(
	ctx context.Context,
	cfg Config,
	out, outErr io.Writer,
) error {
	slog.Info("tofu: deploy infrastructure")

	tofuRunner := NewTofuRunner(cfg, out, outErr)

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
	cfg Config,
	action string,
	out, outErr io.Writer,
) error {
	slog.Info("tofu: change infrastructure")

	tofuRunner := NewTofuRunner(cfg, out, outErr)

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
	cfg Config,
	out, outErr io.Writer,
) error {
	slog.Info("tofu: destroy infrastructure")

	tofuRunner := NewTofuRunner(cfg, out, outErr)

	return tofuRunner.Destroy(ctx, cfg.StateFile())
}
