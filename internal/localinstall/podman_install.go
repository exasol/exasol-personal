// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT
package localinstall

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/runtimeartifacts"
)

const (
	nanoImageOverridePathEnv  = "EXASOL_LOCAL_NANO_IMAGE_OVERRIDE_PATH"
	exasolNanoImageResourceID = "exasol-nano-image"
	loadedImagePrefix         = "Loaded image:"
	dataDirMode               = 0o750
	nanoInternalDBPort        = 8563
	nanoShmSize               = "512mb"
	nanoPIDsLimit             = "-1"
	nanoSecurityOpt           = "unmask=ALL"
	nanoRestartPolicy         = "always"
	nanoDataMountTarget       = "/exa:Z"
	podmanDiagnosticsTimeout  = 10 * time.Second
	nanoVersionCheckDefault   = 86400
	nanoVersionCheckMin       = 60
	nanoVersionCheckMax       = 604800
	nanoVersionCheckRetryMax  = 86400
)

type PodmanInstall struct {
	deployment    config.DeploymentDir
	manager       *runtimeartifacts.Manager
	runtimeExec   []string
	slcStagingDir string
	slcStatusPath string
}

func NewPodmanInstall(
	deployment config.DeploymentDir,
	manager *runtimeartifacts.Manager,
	runtimeExec []string,
	slcStagingDir string,
	slcStatusPath string,
) *PodmanInstall {
	return &PodmanInstall{
		deployment:    deployment,
		manager:       manager,
		runtimeExec:   runtimeExec,
		slcStagingDir: slcStagingDir,
		slcStatusPath: slcStatusPath,
	}
}

func (install *PodmanInstall) Start(
	ctx context.Context,
	out, outErr io.Writer,
	startConfig StartConfig,
) error {
	if err := install.validateStartConfig(startConfig); err != nil {
		return err
	}

	containerName, err := getContainerName(install.deployment)
	if err != nil {
		return err
	}
	if err := install.recoverIncompleteInitialCreate(
		ctx, out, outErr, containerName, startConfig.DataDir,
	); err != nil {
		return err
	}
	running, err := install.containerRunning(ctx, outErr, containerName)
	if err != nil {
		return err
	}
	if running {
		return nil
	}

	if err := os.MkdirAll(startConfig.DataDir, dataDirMode); err != nil {
		return fmt.Errorf("failed to create Nano data directory %s: %w", startConfig.DataDir, err)
	}
	freshDeployment, err := isFreshDeployment(startConfig.DataDir)
	if err != nil {
		return err
	}
	if freshDeployment {
		if err := removeStaleNanoTLSFiles(startConfig.DataDir); err != nil {
			return err
		}
	}

	imagePath, err := install.resolveImagePath(ctx)
	if err != nil {
		return install.failureWithDiagnostics(
			ctx, outErr, containerName, fmt.Errorf("failed to resolve Nano image: %w", err),
		)
	}
	loadOutput, err := install.runPodmanOutput(ctx, out, outErr, "load", "-i", imagePath)
	if err != nil {
		return install.failureWithDiagnostics(ctx, outErr, containerName,
			fmt.Errorf("failed to load Nano image %s: %w", imagePath, err))
	}
	loadedImage, err := parseLoadedImage(loadOutput)
	if err != nil {
		return install.failureWithDiagnostics(ctx, outErr, containerName, err)
	}

	imageTag := "localhost/" + containerName + ":latest"
	if err := install.runPodman(ctx, out, outErr, "tag", loadedImage, imageTag); err != nil {
		return install.failureWithDiagnostics(ctx, outErr, containerName,
			fmt.Errorf("failed to tag Nano image %s as %s: %w", loadedImage, imageTag, err))
	}
	availableSLCs, err := install.materializeSLCs(ctx, out, outErr, containerName, startConfig.SLCs)
	if err != nil {
		return err
	}
	install.pruneUnreferencedSLCImages(ctx, outErr, startConfig.SLCs)

	args := []string{
		"run", "-d", "--replace",
		"--name", containerName,
		"--shm-size=" + nanoShmSize,
		"--pids-limit=" + nanoPIDsLimit,
		"--security-opt", nanoSecurityOpt,
		"--restart", nanoRestartPolicy,
		"-p", fmt.Sprintf("%d:%d", startConfig.ContainerDBPort, nanoInternalDBPort),
		"-v", startConfig.DataDir + ":" + nanoDataMountTarget,
	}
	for _, slc := range availableSLCs {
		args = append(args,
			"--mount",
			fmt.Sprintf("type=image,source=%s,destination=%s", slc.Image, slc.Target),
		)
	}
	if startConfig.VersionCheck.Enabled {
		args = append(args, "-e", "VERSION_CHECK_IDENTITY="+startConfig.VersionCheck.Identity)
	}
	args = append(args, imageTag, "init")
	if freshDeployment && len(startConfig.InitParams) > 0 {
		args = append(args, "params="+strings.Join(startConfig.InitParams, " "))
	}
	args = append(args, nanoVersionCheckInitArgs(startConfig.VersionCheck)...)
	if err := install.runPodman(ctx, out, outErr, args...); err != nil {
		return install.failureWithDiagnostics(ctx, outErr, containerName,
			fmt.Errorf("failed to start Nano container %s: %w", containerName, err))
	}

	return nil
}

func (install *PodmanInstall) Stop(ctx context.Context, out, outErr io.Writer) error {
	containerName, err := getContainerName(install.deployment)
	if err != nil {
		return err
	}
	if err := install.runPodman(
		ctx,
		out,
		outErr,
		"rm",
		"--force",
		"--ignore",
		containerName,
	); err != nil {
		return fmt.Errorf("failed to remove Nano container %s: %w", containerName, err)
	}

	return nil
}

func (install *PodmanInstall) Status(
	ctx context.Context,
	_, outErr io.Writer,
) (*InstallStatus, error) {
	containerName, err := getContainerName(install.deployment)
	if err != nil {
		return nil, err
	}
	running, err := install.containerRunning(ctx, outErr, containerName)
	if err != nil {
		return nil, err
	}

	return &InstallStatus{Running: running}, nil
}

func (install *PodmanInstall) Destroy(ctx context.Context, out, outErr io.Writer) error {
	return install.Stop(ctx, out, outErr)
}

func (install *PodmanInstall) failureWithDiagnostics(
	ctx context.Context,
	outErr io.Writer,
	containerName string,
	failure error,
) error {
	install.writePodmanDiagnostics(ctx, outErr, containerName)

	return failure
}

func (install *PodmanInstall) writePodmanDiagnostics(
	ctx context.Context,
	outErr io.Writer,
	containerName string,
) {
	diagnosticOut := outErr
	if diagnosticOut == nil {
		diagnosticOut = io.Discard
	}
	diagnosticCtx, cancel := context.WithTimeout(
		context.WithoutCancel(ctx),
		podmanDiagnosticsTimeout,
	)
	defer cancel()

	_, _ = fmt.Fprintf(diagnosticOut, "Podman diagnostics for %s:\n", containerName)
	commands := [][]string{
		{"info"},
		{"ps", "-a"},
		{"container", "inspect", containerName},
		{"logs", containerName},
	}
	for _, command := range commands {
		_, _ = fmt.Fprintf(diagnosticOut, "$ podman %s\n", strings.Join(command, " "))
		if err := install.runPodman(
			diagnosticCtx, diagnosticOut, diagnosticOut, command...,
		); err != nil {
			_, _ = fmt.Fprintf(diagnosticOut, "diagnostic command failed: %v\n", err)
		}
	}
}

func (install *PodmanInstall) validateStartConfig(startConfig StartConfig) error {
	if startConfig.ContainerDBPort <= 0 || startConfig.ContainerDBPort > 65535 {
		return fmt.Errorf("invalid Nano published DB port %d", startConfig.ContainerDBPort)
	}
	if strings.TrimSpace(startConfig.DataDir) == "" {
		return errors.New("nano data directory is required")
	}
	if startConfig.VersionCheck.Enabled {
		for name, value := range map[string]string{
			"URL":              startConfig.VersionCheck.URL,
			"identity":         startConfig.VersionCheck.Identity,
			"operating system": startConfig.VersionCheck.OperatingSystem,
		} {
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf("nano version-check %s is required when enabled", name)
			}
		}
	}
	for index, slc := range startConfig.SLCs {
		if strings.TrimSpace(slc.Image) == "" {
			return fmt.Errorf("nano SLC %d image is required", index)
		}
		if strings.TrimSpace(slc.Target) == "" {
			return fmt.Errorf("nano SLC %d target is required", index)
		}
	}
	if startConfig.SLCs != nil {
		if strings.TrimSpace(install.slcStatusPath) == "" {
			return errors.New("SLC status path is required for SLC-aware startup")
		}
		if strings.TrimSpace(install.slcStagingDir) == "" {
			return errors.New("SLC staging directory is required for SLC-aware startup")
		}
	}

	return nil
}

func nanoVersionCheckInitArgs(versionCheck VersionCheckConfig) []string {
	if !versionCheck.Enabled {
		return []string{"VERSION_CHECK_ENABLED=0"}
	}
	interval := versionCheck.IntervalSeconds
	if interval == 0 {
		interval = nanoVersionCheckDefault
	}
	interval = clamp(interval, nanoVersionCheckMin, nanoVersionCheckMax)
	retryInterval := clamp(interval, nanoVersionCheckMin, nanoVersionCheckRetryMax)

	return []string{
		"VERSION_CHECK_ENABLED=1",
		"VERSION_CHECK_ENDPOINT=" + versionCheck.URL,
		fmt.Sprintf("VERSION_CHECK_INTERVAL_SEC=%d", interval),
		fmt.Sprintf("VERSION_CHECK_RETRY_INTERVAL_SEC=%d", retryInterval),
		"VERSION_CHECK_OPERATING_SYSTEM=" + versionCheck.OperatingSystem,
	}
}

func clamp(value, minimum, maximum int) int {
	return min(max(value, minimum), maximum)
}

func isFreshDeployment(dataDir string) (bool, error) {
	configPath := filepath.Join(dataDir, "exasol.conf")
	_, err := os.Stat(configPath)
	if err == nil {
		return false, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}

	return false, fmt.Errorf("failed to inspect Nano configuration %s: %w", configPath, err)
}

func parseLoadedImage(output string) (string, error) {
	var loadedImage string
	for line := range strings.SplitSeq(output, "\n") {
		_, image, found := strings.Cut(line, loadedImagePrefix)
		if !found {
			continue
		}
		image = strings.TrimSpace(image)
		if image == "" {
			continue
		}
		if loadedImage != "" && loadedImage != image {
			return "", errors.New("podman load reported multiple Nano images")
		}
		loadedImage = image
	}
	if loadedImage == "" {
		return "", errors.New("could not determine the image reference reported by podman load")
	}

	return loadedImage, nil
}

func (install *PodmanInstall) containerRunning(
	ctx context.Context,
	outErr io.Writer,
	containerName string,
) (bool, error) {
	exists, err := install.containerExists(ctx, outErr, containerName)
	if err != nil || !exists {
		return false, err
	}
	output, err := install.runPodmanOutput(
		ctx,
		nil,
		outErr,
		"container",
		"inspect",
		"--format",
		"{{.State.Running}}",
		containerName,
	)
	if err != nil {
		return false, fmt.Errorf("failed to inspect Nano container %s: %w", containerName, err)
	}
	running, err := strconv.ParseBool(strings.TrimSpace(output))
	if err != nil {
		return false, fmt.Errorf(
			"failed to parse running state for Nano container %s from %q: %w",
			containerName,
			strings.TrimSpace(output),
			err,
		)
	}

	return running, nil
}

func (install *PodmanInstall) containerExists(
	ctx context.Context,
	outErr io.Writer,
	containerName string,
) (bool, error) {
	err := install.runPodman(
		ctx,
		nil,
		outErr,
		"container",
		"exists",
		containerName,
	)
	if err == nil {
		return true, nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) && exitError.ExitCode() == 1 {
		return false, nil
	}

	return false, fmt.Errorf("failed to find Nano container %s: %w", containerName, err)
}

func (install *PodmanInstall) resolveImagePath(ctx context.Context) (string, error) {
	if override := strings.TrimSpace(os.Getenv(nanoImageOverridePathEnv)); override != "" {
		return override, nil
	}
	if install.manager == nil {
		return "", errors.New("nano runtime artifact manager is required")
	}

	return install.manager.Request(ctx, exasolNanoImageResourceID)
}

func (install *PodmanInstall) runPodman(
	ctx context.Context,
	out, outErr io.Writer,
	arg ...string,
) error {
	cmd := install.command(ctx, "podman", arg...)
	cmd.Stdout = out
	cmd.Stderr = outErr

	return cmd.Run()
}

func (install *PodmanInstall) runPodmanOutput(
	ctx context.Context,
	out, outErr io.Writer,
	arg ...string,
) (string, error) {
	cmd := install.command(ctx, "podman", arg...)
	var stdout bytes.Buffer
	if out == nil {
		cmd.Stdout = &stdout
	} else {
		cmd.Stdout = io.MultiWriter(out, &stdout)
	}
	cmd.Stderr = outErr

	err := cmd.Run()

	return stdout.String(), err
}

func (install *PodmanInstall) command(ctx context.Context, name string, arg ...string) *exec.Cmd {
	cmdLine := make([]string, 0, len(install.runtimeExec)+len(arg)+1)
	cmdLine = append(cmdLine, install.runtimeExec...)
	cmdLine = append(cmdLine, name)
	cmdLine = append(cmdLine, arg...)

	return exec.CommandContext(ctx, cmdLine[0], cmdLine[1:]...)
}

func getContainerName(deployment config.DeploymentDir) (string, error) {
	launcherState, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		return "", err
	}
	deploymentID := strings.TrimSpace(launcherState.DeploymentId)
	if deploymentID == "" {
		return "", errors.New("deployment state is missing deployment ID")
	}

	return "exasol-db-" + deploymentID, nil
}
