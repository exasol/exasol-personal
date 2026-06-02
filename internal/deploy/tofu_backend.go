// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/presets"
	"github.com/exasol/exasol-personal/internal/task_runner"
	"github.com/exasol/exasol-personal/internal/tofu"
	"github.com/exasol/exasol-personal/internal/util"
	"github.com/zclconf/go-cty/cty"
)

type tofuBackend struct{}

func (tofuBackend) ValidateEnvironment() error {
	return nil
}

func (tofuBackend) SetupWorkspace(
	_ context.Context,
	deployment config.DeploymentDir,
	manifest *presets.InfrastructureManifest,
) error {
	if manifest.Tofu == nil {
		slog.Info("tofu: no configuration defined; skipping workspace setup")
		return nil
	}

	tofuCfg := tofu.NewTofuConfigFromDeployment(deployment.Root(), *manifest.Tofu)

	return tofu.SetupWorkspace(tofuCfg)
}

func (tofuBackend) Configure(
	_ context.Context,
	deployment config.DeploymentDir,
	manifest *presets.InfrastructureManifest,
	values []DeploymentConfigValue,
) error {
	if manifest.Tofu == nil {
		slog.Info("tofu: no configuration defined; skipping configuration")
		return nil
	}

	tofuCfg := tofu.NewTofuConfigFromDeployment(deployment.Root(), *manifest.Tofu)

	return tofu.Configure(tofuCfg, configValuesRawMap(values))
}

func (tofuBackend) ReadConfiguration(
	deployment config.DeploymentDir,
	manifest *presets.InfrastructureManifest,
) ([]DeploymentConfigValue, error) {
	if manifest.Tofu == nil {
		return []DeploymentConfigValue{}, nil
	}

	tofuCfg := tofu.NewTofuConfigFromDeployment(deployment.Root(), *manifest.Tofu)
	variableData, err := os.ReadFile(tofuCfg.VariablesFile())
	if err != nil {
		return nil, err
	}
	defaults, err := tofu.ParseVarFile(variableData, tofuCfg.VariablesFile())
	if err != nil {
		return nil, err
	}

	currentValues := map[string]cty.Value{}
	tfvarsData, err := os.ReadFile(tofuCfg.VarsOutputFile())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err == nil {
		currentValues, err = tofu.ParseVarsValuesFile(tfvarsData, tofuCfg.VarsOutputFile())
		if err != nil {
			return nil, err
		}
	}

	values := make([]DeploymentConfigValue, 0, len(defaults))
	for name, variable := range defaults {
		if variable == nil || isReservedInfrastructureVariableName(name) {
			continue
		}
		value := variable.Value
		if current, ok := currentValues[name]; ok {
			value = current
		}
		scalar, err := ctyScalarToGoValue(value)
		if err != nil {
			return nil, fmt.Errorf(
				"invalid current value for infrastructure variable %q: %w",
				name,
				err,
			)
		}
		defaultScalar, err := ctyScalarToGoValue(variable.Value)
		if err != nil {
			return nil, fmt.Errorf(
				"invalid default value for infrastructure variable %q: %w",
				name,
				err,
			)
		}
		rawValue, err := ctyScalarToRawString(value)
		if err != nil {
			return nil, fmt.Errorf(
				"invalid current value for infrastructure variable %q: %w",
				name,
				err,
			)
		}
		rawDefault, err := ctyScalarToRawString(variable.Value)
		if err != nil {
			return nil, fmt.Errorf(
				"invalid default value for infrastructure variable %q: %w",
				name,
				err,
			)
		}
		values = append(values, DeploymentConfigValue{
			Name:         configOptionDisplayName(name),
			Scope:        ConfigScopeInfrastructure,
			Type:         variable.Type,
			Value:        scalar,
			Default:      defaultScalar,
			VariableName: name,
			RawValue:     rawValue,
			RawDefault:   rawDefault,
		})
	}

	return values, nil
}

func (tofuBackend) OpenHostShell(
	ctx context.Context,
	deployment config.DeploymentDir,
	selectedNode string,
) error {
	sshRemote, err := sshRemoteForNodeUnsafe(deployment, selectedNode)
	if err != nil {
		return err
	}

	return sshRemote.Shell(ctx, os.Stdout, os.Stderr)
}

func ctyScalarToRawString(value cty.Value) (string, error) {
	if !value.IsWhollyKnown() || value.IsNull() {
		return "", errors.New("value is unknown or null")
	}
	switch {
	case value.Type() == cty.String:
		return value.AsString(), nil
	case value.Type() == cty.Bool:
		return strconv.FormatBool(value.True()), nil
	case value.Type() == cty.Number:
		float := value.AsBigFloat()

		return float.Text('f', -1), nil
	default:
		return "", fmt.Errorf("unsupported scalar type %s", value.Type().FriendlyName())
	}
}

func ctyScalarToGoValue(value cty.Value) (any, error) {
	raw, err := ctyScalarToRawString(value)
	if err != nil {
		return nil, err
	}
	switch {
	case value.Type() == cty.String:
		return raw, nil
	case value.Type() == cty.Bool:
		return value.True(), nil
	case value.Type() == cty.Number:
		parsed, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return nil, err
		}

		return parsed, nil
	default:
		return nil, fmt.Errorf("unsupported scalar type %s", value.Type().FriendlyName())
	}
}

func (tofuBackend) OpenCOSShell(ctx context.Context, deployment config.DeploymentDir) error {
	sshRemote, err := sshRemoteForNodeUnsafe(deployment, "n11")
	if err != nil {
		return err
	}

	cosCommand := "/usr/bin/env bash /opt/exasol_launcher/scripts/connectCos.sh"

	return sshRemote.RunInteractiveCommand(ctx, cosCommand, os.Stdout, os.Stderr)
}

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

	if err := tofu.Initialize(ctx, tofuCfg,
		stdOutWriter, stdErrWriter, tofuLockfileMode); err != nil {
		logBuffer.ReplayLogMessages(ctx)
		slog.Error("tofu: failed to init", "error", err)

		return err
	}

	if err := tofu.Plan(ctx, tofuCfg, stdOutWriter, stdErrWriter); err != nil {
		logBuffer.ReplayLogMessages(ctx)
		slog.Error("tofu: failed to plan")

		return err
	}

	if err := tofu.ApplyPlan(ctx, tofuCfg, stdOutWriter, stdErrWriter); err != nil {
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
	output := util.CombineWriters(logBuffer, out)
	errOutput := util.CombineWriters(logBuffer, outErr)
	if err := tofu.Initialize(ctx, tofuCfg, output, errOutput, tofu.LockfileReadonly); err != nil {
		logBuffer.ReplayLogMessages(ctx)
		slog.Error("tofu: failed to init before destroy", "error", err)

		return err
	}

	if err := tofu.Destroy(
		ctx,
		tofuCfg,
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
		tofuCfg,
		startStopArg,
		out,
		outErr,
	); err != nil {
		slog.Error("Tofu Apply Failed:", "error", err.Error())
		return err
	}

	return nil
}
