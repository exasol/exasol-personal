// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localinstall"
)

// containerEngineMarkerFileName records the engine a direct-host deployment
// was first prepared with. It lives in the runtime directory, so destroying
// the deployment removes it and the next deploy selects again.
const containerEngineMarkerFileName = "container-engine.json"

//nolint:tagliatelle // Marker file schema is ours; keeping it a single lowercase field.
type containerEngineMarker struct {
	Engine localinstall.ContainerEngine `json:"engine"`
}

// RecordedContainerEngine reports the engine a deployment was first prepared
// with. Database data without a record was created before engines were
// recorded, when Podman was the only engine. Other files left without a record
// say nothing about the engine, as after a partly completed destroy.
func RecordedContainerEngine(
	deployment config.DeploymentDir,
) (localinstall.ContainerEngine, bool, error) {
	paths := newRuntimePaths(deployment)
	data, err := os.ReadFile(containerEngineMarkerPath(paths))
	if err == nil {
		var marker containerEngineMarker
		if err := json.Unmarshal(data, &marker); err != nil || marker.Engine == "" {
			return "", false, fmt.Errorf(
				"invalid container engine record %s", containerEngineMarkerPath(paths),
			)
		}

		return marker.Engine, true, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", false, fmt.Errorf("failed to read the container engine record: %w", err)
	}
	if _, err := os.Stat(filepath.Join(paths.WorkDir, nanoDataDirName)); err == nil {
		return localinstall.EnginePodman, true, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", false, fmt.Errorf("failed to inspect the local database data: %w", err)
	}

	return "", false, nil
}

func recordContainerEngine(paths runtimePaths, engine localinstall.ContainerEngine) error {
	if _, err := os.Stat(containerEngineMarkerPath(paths)); err == nil {
		return nil
	}
	data, err := json.Marshal(containerEngineMarker{Engine: engine})
	if err != nil {
		return err
	}
	if err := config.WriteFileAtomically(
		containerEngineMarkerPath(paths), data, markerFileMode,
	); err != nil {
		return fmt.Errorf("failed to record the container engine: %w", err)
	}

	return nil
}

func containerEngineMarkerPath(paths runtimePaths) string {
	return filepath.Join(paths.WorkDir, containerEngineMarkerFileName)
}

// DetectContainerEngine picks the engine `auto` resolves to: the first engine
// in order of preference that is installed, or Podman, which the launcher can
// install, when none is.
func DetectContainerEngine(
	ctx context.Context,
	preference []localinstall.ContainerEngine,
) localinstall.ContainerEngine {
	for _, engine := range preference {
		if containerEngineInstalled(ctx, engine) {
			return engine
		}
	}

	return localinstall.EnginePodman
}

func containerEngineInstalled(ctx context.Context, engine localinstall.ContainerEngine) bool {
	switch engine {
	case localinstall.EnginePodman:
		return podmanInstalled(ctx)
	case localinstall.EngineDocker:
		return dockerInstalled(ctx)
	default:
		return false
	}
}

// podmanInstalled also consults the registry on Windows, because a Podman the
// launcher installed earlier is missing from the PATH of terminals that were
// already open.
func podmanInstalled(ctx context.Context) bool {
	if podmanOnPath() {
		return true
	}

	return runtime.GOOS == "windows" && ensurePodmanResolvable(ctx) == nil
}

// dockerInstalled accepts a docker CLI that is not Podman's Docker-compatible
// command and whose daemon, if it answers, runs Linux containers. A daemon
// that does not answer still counts, so the user learns why Docker cannot be
// used instead of being moved to Podman silently.
func dockerInstalled(ctx context.Context) bool {
	if _, err := exec.LookPath(dockerCommand); err != nil {
		return false
	}
	if dockerIsPodman(ctx) {
		return false
	}
	probe, err := probeDockerDaemon(ctx)

	return err != nil || probe.osType == dockerLinuxOSType
}

func dockerIsPodman(ctx context.Context) bool {
	probeCtx, cancel := context.WithTimeout(ctx, dockerProbeTimeout)
	defer cancel()
	//nolint:gosec // fixed command and arguments
	output, err := exec.CommandContext(probeCtx, dockerCommand, "--version").CombinedOutput()

	return err == nil && strings.Contains(strings.ToLower(string(output)), "podman")
}
