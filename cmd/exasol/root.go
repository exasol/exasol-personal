// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/lmittmann/tint"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const (
	rootCmdShortDesc = `Exasol Personal: https://github.com/exasol/exasol-personal`

	rootCmdLongDesc = rootCmdShortDesc + `

Getting Started:
  To create and run an Exasol deployment, run "exasol install <infra preset name-or-path>".
	This single command initializes your deployment directory, prepares the selected infrastructure,
	and installs the database. It uses either a built-in infrastructure preset or a custom preset
	at a path you provide. Built-in presets are: local, aws, azure, exoscale, and stackit.

	Quick start: run "exasol install local" for a local deployment, then use "exasol status",
	"exasol connect", "exasol stop", and "exasol start" to manage its lifecycle.

	If you do not pass --deployment-dir and are not already inside a deployment directory,
	Exasol Personal uses ~/.exasol/personal/deployments/default. Pass --deployment-dir
	to override the active deployment directory.

	Note: Cloud presets require provider credentials in your environment.
  Use "exasol init --help", "exasol install --help", or "exasol presets list" to see the preset
  compatibility matrix.

  AI agent skills: https://github.com/exasol-labs/exasol-agent-skills`

	rootCmdExample = `  exasol install local`

	rootCmdGroupEssential = "essential"
	rootCmdGroupLifecycle = "lifecycle"
)

var logLevelMap = map[string]slog.Level{
	"debug": slog.LevelDebug,
	"info":  slog.LevelInfo,
	"warn":  slog.LevelWarn,
	"error": slog.LevelError,
	"":      slog.LevelInfo, // default to info
}

var ErrInvalidLogLevel = errors.New("invalid log level")

var rootCmd = &cobra.Command{
	Use:           "exasol",
	Short:         rootCmdShortDesc,
	Long:          rootCmdLongDesc,
	Example:       rootCmdExample,
	SilenceErrors: true,
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		// Root-level pre-run is the single place where we enforce cross-cutting concerns.
		// Design decision: keep this centralized so individual commands don't have to
		// remember to repeat it (and so user-visible behavior stays consistent).
		if err := setupLogging(); err != nil {
			return err
		}
		if err := resolveDeploymentDirForCommand(cmd, commonFlags); err != nil {
			return err
		}
		deployment := commonFlags.Deployment()

		// Deployment-directory compatibility is enforced centrally and only for commands
		// that declare it via annotations.
		err := enforceDeploymentDirectoryCompatibility(cmd, deployment)
		if err != nil {
			return err
		}

		if !deploymentLogSessionStartsAfterInit(cmd) {
			if err := setupDeploymentLogSession(cmd, deployment); err != nil {
				return err
			}
		}

		// Best-effort version update hint (non-blocking; terminal-only when available).
		// Design decision: never block commands on this.
		if cmd.Name() != "version" && !cmd.Hidden {
			maybeAddVersionUpdateHint(cmd, deployment)
		}

		return nil
	},
}

func setupLogging() error {
	var terminalHandler slog.Handler

	selectedLevel, ok := logLevelMap[commonFlags.LogLevel]
	if !ok {
		return fmt.Errorf("%w: \"%s\"", ErrInvalidLogLevel, commonFlags.LogLevel)
	}

	if term.IsTerminal(int(os.Stderr.Fd())) {
		levelVar := slog.LevelVar{}
		levelVar.Set(selectedLevel)
		// Design decision: when attached to a terminal, prefer human-friendly logs.
		terminalHandler = tint.NewHandler(os.Stderr, &tint.Options{
			Level: &levelVar, TimeFormat: time.DateTime,
		})
	} else {
		terminalHandler = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
			Level: selectedLevel,
		})
	}

	slog.SetDefault(slog.New(newRoutingHandler(terminalHandler, globalDeploymentFileSink)))

	slog.Debug(
		"using log level",
		"log_level", commonFlags.LogLevel,
		"level", logLevelMap[commonFlags.LogLevel],
	)

	return nil
}

// addHelpFlag adds the help flag to the command and all its children.
func addHelpFlag(cmd *cobra.Command) {
	cmd.Flags().BoolP("help", "h", false, "Help for "+cmd.Name())

	for _, child := range cmd.Commands() {
		addHelpFlag(child)
	}
}

func Execute() error {
	resetTerminalMessages()
	registerLogLevelFlag(rootCmd, commonFlags)

	// Register infrastructure variable flags only for commands that need them.
	// This must happen before Cobra parses arguments.
	if err := prepareInfrastructureVariableFlags(os.Args[1:]); err != nil {
		return err
	}
	if err := prepareInstallationVariableFlags(os.Args[1:]); err != nil {
		return err
	}

	// Customize usage/help formatting.
	rootCmd.SetUsageTemplate(customUsageTemplate)
	rootCmd.SetHelpTemplate(
		"{{with (or .Long .Short)}}{{. | trimTrailingWhitespaces}}\n\n{{end}}{{.UsageString}}",
	)

	rootCmd.AddGroup(&cobra.Group{
		ID:    rootCmdGroupEssential,
		Title: "Essential Commands:",
	})

	rootCmd.AddGroup(&cobra.Group{
		ID:    rootCmdGroupLifecycle,
		Title: "Lifecycle Commands:",
	})

	// We add the help flag explicitly because we
	// want to have the "Usage" text be capitalized.
	addHelpFlag(rootCmd)

	err := rootCmd.Execute()
	runDeploymentLogCleanup()
	if err == nil {
		printTerminalMessages()
	}

	return err
}

func maybeAddVersionUpdateHint(cmd *cobra.Command, deployment config.DeploymentDir) {
	result, err := deploy.PerformSilentVersionCheck(
		cmd.Context(),
		deployment,
		CurrentLauncherVersion,
	)
	if err != nil {
		slog.Debug("launcher version update check failed", "error", err)
		return
	}
	if !result.Checked || !result.UpdateAvailable {
		return
	}

	addTerminalNotice(fmt.Sprintf(
		"A new version of Exasol Personal is available: %s. "+
			"Run `exasol version --latest` for more details.",
		result.LatestVersion,
	))
}
