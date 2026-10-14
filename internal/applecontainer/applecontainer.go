// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

// Package applecontainer wraps the two ways the local runtime can install
// Apple's `container` CLI on behalf of the local-apple-container preset: through
// Homebrew when it is available, and through Apple's own signed installer
// package otherwise, so a Homebrew-less Mac is not left without an install path.
package applecontainer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/exasol/exasol-personal/internal/runtimeartifacts"
)

const (
	// CLIBinary is the Apple `container` executable name.
	CLIBinary = "container"
	// BrewFormula is the Homebrew formula that installs the CLI.
	BrewFormula = "container"
	// InstallerResourceID pins Apple's signed installer package in
	// assets/resources.yaml, used when Homebrew is not available.
	InstallerResourceID = "apple-container-installer"
)

// brewBinary and sudoBinary are overridable so tests can stage shims on PATH
// under a stable name without touching the real tools.
var (
	brewBinary = "brew"
	sudoBinary = "sudo"
)

// ErrCLINotFound is returned when the `container` CLI is not on PATH.
var ErrCLINotFound = errors.New("the `container` CLI is not available on PATH")

// ErrBrewNotFound is returned when Homebrew is not available, so callers
// fall back to the signed installer package.
var ErrBrewNotFound = errors.New("Homebrew is not available on PATH")

// LookupCLI returns ErrCLINotFound if `container` is not on PATH, and nil
// otherwise.
func LookupCLI() error {
	if _, err := exec.LookPath(CLIBinary); err != nil {
		return fmt.Errorf("%w: %w", ErrCLINotFound, err)
	}

	return nil
}

// LookupBrew returns ErrBrewNotFound if Homebrew is not on PATH, and nil
// otherwise. Callers use it to decide between the Homebrew and signed
// installer package paths before requesting approval for either.
func LookupBrew() error {
	if _, err := exec.LookPath(brewBinary); err != nil {
		return fmt.Errorf("%w: %w", ErrBrewNotFound, err)
	}

	return nil
}

// BrewInstallArgs returns the argv (without the leading "brew" executable
// name) that InstallViaBrew invokes.
func BrewInstallArgs() []string {
	return []string{"install", BrewFormula}
}

// BrewInstallCommand returns the exact "brew ..." command InstallViaBrew
// will execute, formatted as a single line, shown to the user before approval.
func BrewInstallCommand() string {
	return brewBinary + " " + strings.Join(BrewInstallArgs(), " ")
}

// InstallViaBrew runs "brew install container" and streams its output to
// out/outErr so the user can see download progress.
func InstallViaBrew(ctx context.Context, out, outErr io.Writer) error {
	if err := LookupBrew(); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, brewBinary, BrewInstallArgs()...)
	cmd.Stdout = out
	cmd.Stderr = outErr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("brew install %s failed: %w", BrewFormula, err)
	}

	return nil
}

// ResolveInstallerPackage resolves the pinned signed installer package to a
// local path, downloading and checksum-verifying it through the resource
// manager if it is not already cached.
func ResolveInstallerPackage(
	ctx context.Context,
	manager *runtimeartifacts.Manager,
) (string, error) {
	if manager == nil {
		return "", errors.New(
			"runtime artifact manager is required to resolve the Apple container installer",
		)
	}

	return manager.Request(ctx, InstallerResourceID)
}

// InstallerCommand returns the exact command InstallViaPackage will execute
// for the resolved installer package path, shown to the user before
// approval. It requires administrator privileges, unlike the Homebrew path.
func InstallerCommand(packagePath string) []string {
	return []string{sudoBinary, "installer", "-pkg", packagePath, "-target", "/"}
}

// InstallViaPackage runs Apple's signed installer package with
// administrator privileges, prompting interactively on the controlling
// terminal for the password if needed, and streams its output to
// out/outErr.
func InstallViaPackage(ctx context.Context, packagePath string, out, outErr io.Writer) error {
	args := InstallerCommand(packagePath)
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = out
	cmd.Stderr = outErr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("installer -pkg %s failed: %w", packagePath, err)
	}

	return nil
}
