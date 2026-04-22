// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"io"
	"log/slog"
	"time"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/presets"
	"github.com/exasol/exasol-personal/internal/task_runner"
	"github.com/exasol/exasol-personal/internal/tofu"
	"github.com/exasol/exasol-personal/internal/util"
)

type tofuBackend struct{}

func (tofuBackend) Deploy(
	ctx context.Context,
	deployment config.DeploymentDir,
	manifest *presets.InfrastructureManifest,
	out, outErr io.Writer,
	tofuLockfileMode TofuLockfileMode,
) error {
	if manifest.Tofu == nil {
		slog.Info("tofu: no configuration defined; skipping")
		return nil
	}

	slog.Info("beginning deployment with tofu")

	tofuCfg := tofu.NewTofuConfigFromDeployment(deployment.Root(), *manifest.Tofu)
	logBuffer := task_runner.NewLogBuffer()
	stdOutWriter := util.CombineWriters(logBuffer, out)
	stdErrWriter := util.CombineWriters(logBuffer, outErr)

	if err := tofu.Initialize(ctx, *tofuCfg,
		stdOutWriter, stdErrWriter, tofuLockfileMode); err != nil {
		logBuffer.ReplayLogMessages(ctx)
		slog.Error("tofu: failed to init", "error", err)

		return err
	}

	if err := tofu.Plan(ctx, *tofuCfg, stdOutWriter, stdErrWriter); err != nil {
		logBuffer.ReplayLogMessages(ctx)
		slog.Error("tofu: failed to plan")

		return err
	}

	if err := tofu.ApplyPlan(ctx, *tofuCfg, stdOutWriter, stdErrWriter); err != nil {
		logBuffer.ReplayLogMessages(ctx)
		slog.Error("tofu: failed to apply")

		return err
	}

	return nil
}

func (tofuBackend) Start(
	ctx context.Context,
	deployment config.DeploymentDir,
	manifest *presets.InfrastructureManifest,
	out, outErr io.Writer,
	waitTimeoutSeconds int,
) error {
	logBuffer := task_runner.NewLogBuffer()
	output := util.CombineWriters(logBuffer, out)
	errOutput := util.CombineWriters(logBuffer, outErr)

	if err := tofuApplyAction(
		ctx,
		deployment,
		manifest,
		"power_state=running",
		output,
		errOutput,
	); err != nil {
		logBuffer.ReplayLogMessages(ctx)
		slog.Error("failed to start deployment")

		return err
	}

	instPollCond := func(ctx context.Context) (bool, error) {
		n11Details, err := Getn11Details(deployment)
		if err != nil {
			return false, err
		}
		if n11Details.Host != "" {
			return true, nil
		}

		if err := tofuApplyAction(
			ctx,
			deployment,
			manifest,
			"",
			output,
			errOutput,
		); err != nil {
			logBuffer.ReplayLogMessages(ctx)
			slog.Error("ApplyAction failed while refreshing", "error", err)

			return false, err
		}

		return false, nil
	}

	waitCtx, cancel := context.WithTimeout(
		ctx,
		time.Duration(InstanceRefreshTimeoutSeconds)*time.Second,
	)
	defer cancel()

	if err := PollWithBackoff(waitCtx, instPollCond, WaitParams{
		InitialBackoff: StartedInitialBackoff,
		MaxBackoff:     StartedMaxBackoff,
		LogPrefix:      "waiting to update EC2 Resources",
	}); err != nil {
		slog.Error("Updated EC2 resources not available in time")
		return err
	}

	if waitTimeoutSeconds <= 0 {
		waitTimeoutSeconds = StartedDefaultTimeoutSeconds
	}

	waitCtx, cancel = context.WithTimeout(ctx, time.Duration(waitTimeoutSeconds)*time.Second)
	defer cancel()

	if err := WaitForDatabaseStarted(waitCtx, deployment); err != nil {
		slog.Error("database did not become operational with timeout", "error", err.Error())
		return err
	}

	return nil
}

func (tofuBackend) Stop(
	ctx context.Context,
	deployment config.DeploymentDir,
	manifest *presets.InfrastructureManifest,
	out, outErr io.Writer,
) error {
	logBuffer := task_runner.NewLogBuffer()

	if err := tofuApplyAction(
		ctx,
		deployment,
		manifest,
		"power_state=stopped",
		util.CombineWriters(logBuffer, out),
		util.CombineWriters(logBuffer, outErr),
	); err != nil {
		logBuffer.ReplayLogMessages(ctx)
		slog.Error("failed to stop the deployment")
		return err
	}

	return nil
}

func (tofuBackend) Destroy(
	ctx context.Context,
	deployment config.DeploymentDir,
	manifest *presets.InfrastructureManifest,
	out, outErr io.Writer,
) error {
	if manifest.Tofu == nil {
		slog.Info("no tofu configuration defined; skipping destroy")
		return nil
	}

	tofuCfg := tofu.NewTofuConfigFromDeployment(deployment.Root(), *manifest.Tofu)
	logBuffer := task_runner.NewLogBuffer()
	if err := tofu.Destroy(
		ctx,
		*tofuCfg,
		util.CombineWriters(logBuffer, out),
		util.CombineWriters(logBuffer, outErr),
	); err != nil {
		logBuffer.ReplayLogMessages(ctx)
		slog.Error("failed to destroy cloud resources")

		return err
	}

	return nil
}

func tofuApplyAction(
	ctx context.Context,
	deployment config.DeploymentDir,
	manifest *presets.InfrastructureManifest,
	startStopArg string,
	out, outErr io.Writer,
) error {
	if manifest.Tofu == nil {
		slog.Info("no tofu configuration defined; skipping apply action")
		return nil
	}

	tofuCfg := tofu.NewTofuConfigFromDeployment(deployment.Root(), *manifest.Tofu)
	if err := tofu.ApplyAction(
		ctx,
		*tofuCfg,
		startStopArg,
		out,
		outErr,
	); err != nil {
		slog.Error("Tofu Apply Failed:", "error", err.Error())
		return err
	}

	return nil
}
