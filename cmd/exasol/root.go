// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/lmittmann/tint"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const (
	rootCmdShortDesc = `Exasol Personal Launcher`

	rootCmdLongDesc = rootCmdShortDesc + `

Getting Started:
  Begin by creating a deployment directory, e.g., with "mkdir deployment", and then change
  into that directory, e.g., with "cd deployment".

  To create and run an Exasol deployment, use the "exasol install" command.
  This single command initializes your deployment directory, provisions cloud infrastructure,
  and installs the database.

  Alternatively, use "exasol init" to set up the deployment directory with configuration
  and then "exasol deploy" to provision infrastructure and install Exasol into it.

  Note: Ensure your cloud provider credentials are configured in your environment before running.`

	rootCmdExample = `  exasol install aws --deployment-dir ./my-exasol`

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

		// Deployment-directory compatibility is enforced centrally and only for commands
		// that declare it via annotations.
		err := enforceDeploymentDirectoryCompatibility(cmd, commonFlags.DeploymentDir)
		if err != nil {
			return err
		}

		// Best-effort version update hint (non-blocking; logs only when an update is available).
		// Design decision: never block commands on this.
		if cmd.Name() != "version" {
			deploy.MaybeLogVersionUpdateHint(
				cmd.Context(), commonFlags.DeploymentDir,
				CurrentLauncherVersion,
			)
		}

		return nil
	},
}

func setupLogging() error {
	var logger *slog.Logger

	selectedLevel, ok := logLevelMap[commonFlags.LogLevel]
	if !ok {
		return fmt.Errorf("%w: \"%s\"", ErrInvalidLogLevel, commonFlags.LogLevel)
	}
	if term.IsTerminal(int(os.Stdout.Fd())) {
		levelVar := slog.LevelVar{}
		levelVar.Set(selectedLevel)
		// Design decision: when attached to a terminal, prefer human-friendly logs.
		logger = slog.New(tint.NewHandler(os.Stderr, &tint.Options{
			Level: &levelVar, TimeFormat: time.DateTime,
		}))
	} else {
		logger = slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
			Level: selectedLevel,
		}))
	}
	slog.SetDefault(logger)

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

	return rootCmd.Execute()
}
