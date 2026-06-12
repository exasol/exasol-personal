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
	resourceID   string
	targetPath   string
	runnerPath   string
	cacheRoot    string
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
		return runPlaceholder(args[1:])
	case "stage":
		return runStage(ctx, args[1:])
	default:
		return fmt.Errorf("unknown subcommand %q", args[0])
	}
}

func runPlaceholder(args []string) error {
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

func runStage(ctx context.Context, args []string) error {
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

	sourcePath, err := resolveRunnerSource(ctx, config)
	if err != nil {
		return err
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

	config := runnerConfig{}
	flags.StringVar(
		&config.resourceID,
		"resource-id",
		defaultResourceID,
		"Resource ID in resources.yaml",
	)
	flags.StringVar(&config.targetPath, "target", defaultGeneratedPath, "Staged runner path")
	flags.StringVar(&config.runnerPath, "runner-path", "", "Existing runner binary path")
	flags.StringVar(&config.cacheRoot, "cache-root", "", "Runtime artifact cache root")
	flags.StringVar(&config.targetGOOS, "goos", targetEnv("GOOS", runtime.GOOS), "Target GOOS")
	flags.StringVar(
		&config.targetGOARCH,
		"goarch",
		targetEnv("GOARCH", runtime.GOARCH),
		"Target GOARCH",
	)
	if err := flags.Parse(args); err != nil {
		return runnerConfig{}, err
	}

	if strings.TrimSpace(config.resourceID) == "" {
		return runnerConfig{}, errors.New("resource-id must not be empty")
	}

	return config, nil
}

func targetEnv(name, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}

	return value
}

func resolveRunnerSource(ctx context.Context, config runnerConfig) (string, error) {
	if source := strings.TrimSpace(config.runnerPath); source != "" {
		return requireFile(source)
	}

	spec, err := runtimeartifacts.ParseSpec(resources.ResourcesYAML)
	if err != nil {
		return "", err
	}

	return resolveRunnerSourceFromSpec(ctx, config, spec)
}

func resolveRunnerSourceFromSpec(
	ctx context.Context,
	config runnerConfig,
	spec runtimeartifacts.ResourceSpec,
) (string, error) {
	manager, err := newResourceManager(config, spec)
	if err != nil {
		return "", err
	}

	return manager.Request(ctx, config.resourceID)
}

func requireFile(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("runner source is a directory: %s", path)
	}

	return path, nil
}

func newResourceManager(
	config runnerConfig,
	spec runtimeartifacts.ResourceSpec,
) (*runtimeartifacts.Manager, error) {
	if strings.TrimSpace(config.cacheRoot) != "" {
		return runtimeartifacts.NewResourceManagerForPlatform(
			spec,
			config.cacheRoot,
			config.targetGOOS,
			config.targetGOARCH,
		), nil
	}

	cache, err := runtimeartifacts.NewDefaultCache()
	if err != nil {
		return nil, err
	}

	return runtimeartifacts.NewResourceManagerWithCacheForPlatform(
		spec,
		cache,
		config.targetGOOS,
		config.targetGOARCH,
	), nil
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

	tmpFile, err := os.CreateTemp(filepath.Dir(targetPath), "runner-*")
	if err != nil {
		return false, err
	}
	tmpPath := tmpFile.Name()

	_, copyErr := io.Copy(tmpFile, source)
	closeErr := tmpFile.Close()
	if copyErr != nil {
		_ = os.Remove(tmpPath)

		return false, copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)

		return false, closeErr
	}
	if err := os.Chmod(tmpPath, executablePerm); err != nil {
		_ = os.Remove(tmpPath)

		return false, err
	}

	_ = os.Remove(targetPath)

	return true, os.Rename(tmpPath, targetPath)
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
