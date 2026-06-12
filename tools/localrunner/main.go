// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/exasol/exasol-personal/assets/resources"
	"github.com/exasol/exasol-personal/internal/runtimeartifacts"
)

const (
	dirPerm              = 0o700
	executablePerm       = 0o700
	defaultResourceID    = "exasol-local-runner"
	defaultGeneratedPath = "assets/localruntimebin/generated/darwin/arm64/mac-runner-aarch64"
	placeholderText      = "placeholder for go:embed"
)

type runnerConfig struct {
	targetPath   string
	runnerPath   string
	targetGOOS   string
	targetGOARCH string
}

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("expected subcommand: placeholder or stage")
	}

	switch args[0] {
	case "placeholder":
		return preparePlaceholder(args[1:])
	case "stage":
		return prepareRunner(ctx, args[1:])
	default:
		return fmt.Errorf("unknown subcommand %q", args[0])
	}
}

func preparePlaceholder(args []string) error {
	flags := flag.NewFlagSet("placeholder", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	targetPath := flags.String("target", defaultGeneratedPath, "Placeholder file path")
	text := flags.String("text", placeholderText, "Placeholder file content")
	if err := flags.Parse(args); err != nil {
		return err
	}

	if _, err := os.Stat(*targetPath); err == nil {
		fmt.Fprintf(os.Stdout, "Exasol Local runner placeholder already exists: %s\n", *targetPath)

		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(*targetPath), dirPerm); err != nil {
		return err
	}

	return os.WriteFile(*targetPath, []byte(*text), executablePerm)
}

func prepareRunner(ctx context.Context, args []string) error {
	config, err := parseRunnerFlags("stage", args)
	if err != nil {
		return err
	}
	if config.targetGOOS != "darwin" || config.targetGOARCH != "arm64" {
		fmt.Fprintf(
			os.Stdout,
			"Skipping Exasol Local runner staging for %s/%s\n",
			config.targetGOOS,
			config.targetGOARCH,
		)

		return nil
	}

	sourcePath := strings.TrimSpace(config.runnerPath)
	if sourcePath != "" {
		info, err := os.Stat(sourcePath)
		if err != nil {
			return err
		}
		if info.IsDir() {
			return fmt.Errorf("runner source is a directory: %s", sourcePath)
		}
	} else {
		spec, err := runtimeartifacts.ParseSpec(resources.ResourcesYAML)
		if err != nil {
			return err
		}
		cache, err := runtimeartifacts.NewDefaultCache()
		if err != nil {
			return err
		}
		manager := runtimeartifacts.NewResourceManagerWithCacheForPlatform(
			spec,
			cache,
			config.targetGOOS,
			config.targetGOARCH,
		)
		sourcePath, err = manager.Request(ctx, defaultResourceID)
		if err != nil {
			return err
		}
	}

	changed, err := copyExecutable(sourcePath, config.targetPath)
	if err != nil {
		return err
	}
	if !changed {
		fmt.Fprintf(os.Stdout, "Exasol Local runner already staged: %s\n", config.targetPath)

		return nil
	}
	fmt.Fprintf(os.Stdout, "Staged Exasol Local runner: %s -> %s\n", sourcePath, config.targetPath)

	return nil
}

func parseRunnerFlags(name string, args []string) (runnerConfig, error) {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	config := runnerConfig{targetGOOS: runtime.GOOS, targetGOARCH: runtime.GOARCH}
	if goos := strings.TrimSpace(os.Getenv("GOOS")); goos != "" {
		config.targetGOOS = goos
	}
	if goarch := strings.TrimSpace(os.Getenv("GOARCH")); goarch != "" {
		config.targetGOARCH = goarch
	}
	flags.StringVar(&config.targetPath, "target", defaultGeneratedPath, "Staged runner path")
	flags.StringVar(&config.runnerPath, "runner-path", "", "Existing runner binary path")
	flags.StringVar(&config.targetGOOS, "goos", config.targetGOOS, "Target GOOS")
	flags.StringVar(&config.targetGOARCH, "goarch", config.targetGOARCH, "Target GOARCH")
	if err := flags.Parse(args); err != nil {
		return runnerConfig{}, err
	}

	return config, nil
}

func copyExecutable(sourcePath, targetPath string) (bool, error) {
	if err := os.MkdirAll(filepath.Dir(targetPath), dirPerm); err != nil {
		return false, err
	}
	if same, err := sameFileContent(sourcePath, targetPath); err != nil {
		return false, err
	} else if same {
		return false, os.Chmod(targetPath, executablePerm)
	}

	source, err := os.Open(sourcePath)
	if err != nil {
		return false, err
	}
	defer func() {
		_ = source.Close()
	}()

	target, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, executablePerm)
	if err != nil {
		return false, err
	}

	_, copyErr := io.Copy(target, source)
	closeErr := target.Close()
	if copyErr != nil {
		return false, copyErr
	}
	if closeErr != nil {
		return false, closeErr
	}

	return true, os.Chmod(targetPath, executablePerm)
}

func sameFileContent(sourcePath, targetPath string) (bool, error) {
	sourceData, err := os.ReadFile(sourcePath)
	if err != nil {
		return false, err
	}
	targetData, err := os.ReadFile(targetPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}

		return false, err
	}

	return bytes.Equal(sourceData, targetData), nil
}
