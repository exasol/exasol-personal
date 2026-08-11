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
	"strings"

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
)

type PodmanInstall struct {
	deployment  config.DeploymentDir
	manager     *runtimeartifacts.Manager
	runtimeExec []string
}

func NewPodmanInstall(
	deployment config.DeploymentDir,
	manager *runtimeartifacts.Manager,
	runtimeExec []string,
) *PodmanInstall {
	return &PodmanInstall{
		deployment:  deployment,
		manager:     manager,
		runtimeExec: runtimeExec,
	}
}

func (install *PodmanInstall) Start(
	ctx context.Context,
	out, outErr io.Writer,
	startConfig StartConfig,
) error {
	if err := validateStartConfig(startConfig); err != nil {
		return err
	}

	containerName, err := getContainerName(install.deployment)
	if err != nil {
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

	imagePath, err := install.resolveImagePath(ctx)
	if err != nil {
		return fmt.Errorf("failed to resolve Nano image: %w", err)
	}
	loadOutput, err := install.runPodmanOutput(ctx, out, outErr, "load", "-i", imagePath)
	if err != nil {
		return fmt.Errorf("failed to load Nano image %s: %w", imagePath, err)
	}
	loadedImage, err := parseLoadedImage(loadOutput)
	if err != nil {
		return err
	}

	imageTag := "localhost/" + containerName + ":latest"
	if err := install.runPodman(ctx, out, outErr, "tag", loadedImage, imageTag); err != nil {
		return fmt.Errorf("failed to tag Nano image %s as %s: %w", loadedImage, imageTag, err)
	}

	args := []string{
		"run", "-d", "--replace",
		"--name", containerName,
		"--shm-size=" + nanoShmSize,
		"--pids-limit=" + nanoPIDsLimit,
		"--security-opt", nanoSecurityOpt,
		"--restart", nanoRestartPolicy,
		"-p", fmt.Sprintf("%d:%d", startConfig.ContainerDBPort, nanoInternalDBPort),
		"-v", startConfig.DataDir + ":" + nanoDataMountTarget,
		imageTag,
		"init",
	}
	if freshDeployment && len(startConfig.InitParams) > 0 {
		args = append(args, "params="+strings.Join(startConfig.InitParams, " "))
	}
	if err := install.runPodman(ctx, out, outErr, args...); err != nil {
		return fmt.Errorf("failed to start Nano container %s: %w", containerName, err)
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

func (install *PodmanInstall) Destroy(ctx context.Context, out, outErr io.Writer) error {
	return install.Stop(ctx, out, outErr)
}

func validateStartConfig(startConfig StartConfig) error {
	if startConfig.ContainerDBPort <= 0 || startConfig.ContainerDBPort > 65535 {
		return fmt.Errorf("invalid Nano published DB port %d", startConfig.ContainerDBPort)
	}
	if strings.TrimSpace(startConfig.DataDir) == "" {
		return errors.New("nano data directory is required")
	}

	return nil
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
	output, err := install.runPodmanOutput(
		ctx,
		nil,
		outErr,
		"ps",
		"--format",
		"{{.Names}}",
	)
	if err != nil {
		return false, fmt.Errorf("failed to inspect Nano container %s: %w", containerName, err)
	}
	for name := range strings.FieldsSeq(output) {
		if name == containerName {
			return true, nil
		}
	}

	return false, nil
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
