// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"context"
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
		if err := setuplogging(); err != nil {
			return err
		}

		// Perform silent version check (non-blocking, only logs if update available)
		// Skip for the version command itself and if disabled
		// And if the directory isn't initialized
		if cmd.Name() != "version" {
			printVersionUpdateHint(cmd.Context())
		}

		return nil
	},
}

func setuplogging() error {
	var logger *slog.Logger

	selectedLevel, ok := logLevelMap[commonFlags.LogLevel]
	if !ok {
		return fmt.Errorf("%w: \"%s\"", ErrInvalidLogLevel, commonFlags.LogLevel)
	}
	if term.IsTerminal(int(os.Stdout.Fd())) {
		levelVar := slog.LevelVar{}
		levelVar.Set(selectedLevel)
		// --log-level is not set and terminal is attached. Use pretty-printing for log messages
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

func printVersionUpdateHint(ctx context.Context) {
	result, err := deploy.PerformSilentVersionCheck(
		ctx,
		commonFlags.DeploymentDir,
		version,
	)
	if err != nil {
		slog.Debug("launcher version update check failed", "error", err)
		return
	}
	if !result.Checked {
		return
	}
	if result.UpdateAvailable {
		slog.Info(
			"A new version of Exasol Personal is available",
			"current", version,
			"latest", result.LatestVersion,
			"info", "Run 'exasol version --latest' for more details",
		)
	}
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
