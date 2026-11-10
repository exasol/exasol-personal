// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"

	"github.com/exasol/exasol-personal/internal/localinstall"
	"github.com/exasol/exasol-personal/internal/winget"
)

const podmanCommand = "podman"

var installWingetPackage = winget.Install

// wingetEngine is how Windows Package Manager installs an engine.
type wingetEngine struct {
	pkg        winget.Package
	binary     string
	name       string
	installURL string
}

var wingetEngines = map[localinstall.ContainerEngine]wingetEngine{
	localinstall.EnginePodman: {
		pkg:        winget.PodmanPackage,
		binary:     podmanCommand,
		name:       podmanCommand,
		installURL: "https://podman.io/",
	},
	localinstall.EngineDocker: {
		pkg:        winget.DockerDesktopPackage,
		binary:     dockerCommand,
		name:       "Docker Desktop",
		installURL: "https://docs.docker.com/desktop/setup/install/windows-install/",
	},
}

// IsContainerEngineInstalled reports whether `auto` would find engine on this
// host.
func IsContainerEngineInstalled(ctx context.Context, engine localinstall.ContainerEngine) bool {
	return containerEngineInstalled(ctx, engine)
}

// ContainerEngineCommandFound reports whether engine's command resolves,
// whether or not `auto` would use it.
func ContainerEngineCommandFound(ctx context.Context, engine localinstall.ContainerEngine) bool {
	switch engine {
	case localinstall.EnginePodman:
		return podmanInstalled(ctx)
	case localinstall.EngineDocker:
		return resolveDocker(ctx, runtime.GOOS == string(HostPlatformWindows)) == nil
	default:
		return false
	}
}

// IsContainerEngineUsable reports whether an installed engine can run a
// deployment as found: Docker must answer with Linux containers, and Podman
// must be installed.
func IsContainerEngineUsable(ctx context.Context, engine localinstall.ContainerEngine) bool {
	switch engine {
	case localinstall.EnginePodman:
		return podmanInstalled(ctx)
	case localinstall.EngineDocker:
		return requireDockerLinuxEngine(
			ctx,
			runtime.GOOS == string(HostPlatformWindows),
			false,
		) == nil
	default:
		return false
	}
}

// ContainerEngineInstallOptions describes how Windows Package Manager installs
// each of engines, in the given order.
func ContainerEngineInstallOptions(
	engines []localinstall.ContainerEngine,
) ([]ContainerEngineOption, error) {
	if err := winget.LookupWinget(); err != nil {
		return nil, fmt.Errorf(
			"no container engine is installed, and Windows Package Manager (winget) is "+
				"unavailable to install one; install App Installer from the Microsoft Store, "+
				"or install Podman or Docker Desktop manually, then re-run this command: %w", err)
	}
	options := make([]ContainerEngineOption, 0, len(engines))
	for _, engine := range engines {
		if _, known := wingetEngines[engine]; known {
			options = append(options, ContainerEngineOption{
				Engine:   engine,
				Commands: []HostCommand{wingetInstallCommand(engine)},
			})
		}
	}

	return options, nil
}

// alternativeContainerEngine is the other engine a deployment could select
// instead of installing engine: one that is usable as found, or missing and
// installable, but not one that is installed and cannot be used. A deployment
// that recorded its engine cannot select another.
func alternativeContainerEngine(
	ctx context.Context,
	options PrepareOptions,
	engine localinstall.ContainerEngine,
) localinstall.ContainerEngine {
	if options.engineRecorded {
		return ""
	}
	for other := range wingetEngines {
		if other == engine {
			continue
		}
		if ContainerEngineCommandFound(ctx, other) && !IsContainerEngineUsable(ctx, other) {
			return ""
		}

		return other
	}

	return ""
}

func wingetInstallCommand(engine localinstall.ContainerEngine) HostCommand {
	return HostCommand{Name: "winget", Args: winget.InstallArgs(wingetEngines[engine].pkg)}
}

// installWithWinget installs engine through Windows Package Manager after
// approval, and makes its command resolvable from this process.
func installWithWinget(
	ctx context.Context,
	options PrepareOptions,
	engine localinstall.ContainerEngine,
) error {
	target, known := wingetEngines[engine]
	if !known {
		return fmt.Errorf("the launcher cannot install %s", engine)
	}
	if err := winget.LookupWinget(); err != nil {
		return fmt.Errorf(
			"%s is required for local deployments but is not installed, and Windows "+
				"Package Manager (winget) is unavailable to install it; install "+
				"App Installer from the Microsoft Store, or install %s manually "+
				"from %s, then re-run this command: %w",
			target.name, target.name, target.installURL, err)
	}

	request := HostChangeRequest{
		Kind:        HostChangeInstallContainerRuntime,
		Engine:      engine,
		Alternative: alternativeContainerEngine(ctx, options, engine),
		Commands:    []HostCommand{wingetInstallCommand(engine)},
	}
	if err := requireHostChangeApproval(ctx, options, request); err != nil {
		return err
	}

	writePreparationProgress(options.Progress, "Installing "+target.name+"...")
	if err := installWingetPackage(
		ctx,
		target.pkg,
		options.Progress,
		options.Progress,
	); err != nil {
		return fmt.Errorf(
			"failed to install %s through Windows Package Manager; install it "+
				"manually from %s, then re-run this command: %w",
			target.name, target.installURL, err)
	}
	if err := winget.RefreshPath(ctx); err != nil {
		return fmt.Errorf("failed to refresh PATH after installing %s: %w", target.name, err)
	}
	if _, err := exec.LookPath(target.binary); err != nil {
		return errors.New(
			"the " + target.name + " installation completed but " + target.binary +
				" is still not on PATH; open a new terminal and re-run this command",
		)
	}

	return nil
}
