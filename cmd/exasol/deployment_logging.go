// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/spf13/cobra"
)

const (
	deploymentLogsDirPermissions            = 0o750
	deploymentLogFilePermissions            = 0o600
	deploymentLogFileName                   = "deployment.log"
	annotationRequiresDeploymentFileLogging = "exasol.requiresDeploymentFileLogging"
	commandInit                             = "init"
	commandInstall                          = "install"
)

var deploymentLogCleanup = func() {}

func requireDeploymentFileLogging(cmd *cobra.Command) {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[annotationRequiresDeploymentFileLogging] = annotationEnabledValue
}

func deploymentFileLoggingIsRequired(cmd *cobra.Command) bool {
	v, ok := cmd.Annotations[annotationRequiresDeploymentFileLogging]
	return ok && v == annotationEnabledValue
}

var startDeploymentLogSession = func(
	_ context.Context,
	commandName string,
	deployment config.DeploymentDir,
) (func(), error) {
	logFilePath := deploymentLogFilePath(deployment)
	logDir := filepath.Dir(logFilePath)
	if err := os.MkdirAll(logDir, deploymentLogsDirPermissions); err != nil {
		slog.Warn(
			"failed to enable deployment file logging; continuing without deployment file logging",
			"deployment_dir", deployment.Root(),
			"error", err.Error(),
		)

		return func() {}, nil
	}

	file, err := os.OpenFile(
		logFilePath,
		os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		deploymentLogFilePermissions,
	)
	if err != nil {
		slog.Warn(
			"failed to open deployment log file; continuing without deployment file logging",
			"path", logFilePath,
			"error", err.Error(),
		)

		return func() {}, nil
	}

	globalDeploymentFileSink.Set(file, slog.LevelDebug)
	writeDeploymentLogBootstrap(file, commandName, deployment)

	slog.Debug("deployment log file", "status", "started", "path", logFilePath)

	return func() {
		slog.Debug("deployment log file", "status", "finished", "path", logFilePath)
		globalDeploymentFileSink.Clear()

		if err := file.Close(); err != nil {
			slog.Warn("failed to close deployment log file", "path",
				logFilePath, "error", err.Error())
		}
	}, nil
}

func setDeploymentLogCleanup(cleanup func()) {
	if cleanup == nil {
		deploymentLogCleanup = func() {}
		return
	}

	deploymentLogCleanup = cleanup
}

func runDeploymentLogCleanup() {
	deploymentLogCleanup()
	deploymentLogCleanup = func() {}
}

func setupDeploymentLogSession(cmd *cobra.Command, deployment config.DeploymentDir) error {
	// Default cleanup is always a no-op, so Execute() can always defer cleanup once.
	setDeploymentLogCleanup(func() {})

	if !deploymentFileLoggingIsRequired(cmd) {
		return nil
	}
	if cmd.Name() == "destroy" && destroyOpts.Remove {
		return nil
	}

	cleanup, err := startDeploymentLogSession(cmd.Context(), cmd.Name(), deployment)
	setDeploymentLogCleanup(cleanup)

	if err != nil {
		// Never fail the command because persistent logging setup failed.
		slog.Warn("failed to set up persistent logging; continuing without deployment file logging",
			"error", err.Error())
	}

	return nil
}

func deploymentLogSessionStartsAfterInit(cmd *cobra.Command) bool {
	switch cmd.Name() {
	case commandInit, commandInstall:
		return true
	default:
		return false
	}
}

func deploymentLogFilePath(deployment config.DeploymentDir) string {
	return deployment.Resolve(deploymentLogFileName)
}

func writeDeploymentLogBootstrap(
	file *os.File,
	commandName string,
	deployment config.DeploymentDir,
) {
	writeBootstrapRecord(file, "deployment log session started",
		slog.String("command", commandName),
		slog.String("deployment_dir", deployment.Root()),
	)
	writeBootstrapRecord(file, "system information",
		slog.String("os", runtime.GOOS),
		slog.String("arch", runtime.GOARCH),
		slog.Int("cpus", runtime.NumCPU()),
		slog.String("go_version", runtime.Version()),
		slog.Int("pid", os.Getpid()),
	)
}

func writeBootstrapRecord(file *os.File, message string, attrs ...slog.Attr) {
	record := slog.NewRecord(time.Now().UTC(), slog.LevelInfo, message, 0)
	record.AddAttrs(attrs...)

	if _, err := file.WriteString(formatFileLogRecord(record)); err != nil {
		slog.Warn("failed to write bootstrap log entry", "error", err.Error())
	}
}
