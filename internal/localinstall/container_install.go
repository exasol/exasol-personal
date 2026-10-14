// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localinstall

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localports"
)

// containerShmSize matches the format Apple's `container` CLI accepts for
// --shm-size (e.g. "512M"), which differs from Podman's "512mb".
const containerShmSize = "512M"

// ErrScriptLanguageContainersUnsupported is returned when a deployment using
// the Apple container runtime has script language containers configured.
// Apple's `container` CLI has no equivalent of Podman's `--mount
// type=image,...,rw=true`, which is how an SLC image is mounted into Nano
// without ever modifying it, so SLC support is explicitly out of scope here
// rather than approximated with a lossier mechanism.
var ErrScriptLanguageContainersUnsupported = errors.New(
	"script language containers are not yet supported on the local-apple-container preset",
)

// AppleContainerInstall drives the Nano lifecycle through Apple's `container`
// CLI directly on the host. It intentionally does not share code with
// PodmanInstall: the two CLIs diverge enough (no --restart, no
// --security-opt, no SLC image mount, different inspect/load output shapes)
// that forcing a shared implementation would risk the Podman path for no
// benefit. See LocalInstall for the shared contract both satisfy.
type AppleContainerInstall struct {
	deployment    config.DeploymentDir
	environment   ExecutionEnvironment
	resolveImage  func(context.Context) (RuntimePath, error)
	slcStagingDir RuntimePath
	slcStatusPath RuntimePath
}

func NewAppleContainerInstallWithEnvironment(
	deployment config.DeploymentDir,
	environment ExecutionEnvironment,
	resolveImage func(context.Context) (RuntimePath, error),
	slcStagingDir RuntimePath,
	slcStatusPath RuntimePath,
) *AppleContainerInstall {
	return &AppleContainerInstall{
		deployment:    deployment,
		environment:   environment,
		resolveImage:  resolveImage,
		slcStagingDir: slcStagingDir,
		slcStatusPath: slcStatusPath,
	}
}

func (install *AppleContainerInstall) Start(
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
	// An interrupted first initialization can leave its password file behind.
	if err := install.RemoveBootstrapPassword(ctx, startConfig.BootstrapDir); err != nil {
		return err
	}

	containerStatus, err := install.inspectContainerStatus(ctx, outErr, containerName)
	if err != nil {
		return err
	}
	if containerStatus.Running {
		return nil
	}
	if containerStatus.Exists {
		// Apple's `container` has no `--replace` flag, so a stopped container
		// left over from a previous start (possibly with stale port or
		// version-check arguments) is removed explicitly before recreating it
		// with the current configuration.
		if err := install.runContainer(ctx, out, outErr, "delete", "--force", containerName); err != nil {
			return install.failureWithDiagnostics(ctx, outErr, containerName,
				fmt.Errorf("failed to remove stale Nano container %s: %w", containerName, err))
		}
	}

	if err := install.environment.MkdirAll(ctx, startConfig.DataDir, dataDirMode); err != nil {
		return fmt.Errorf("failed to create Nano data directory %s: %w", startConfig.DataDir, err)
	}
	freshDeployment, err := install.isFreshDeployment(ctx, startConfig.DataDir)
	if err != nil {
		return err
	}
	if freshDeployment {
		if err := install.removeStaleNanoTLSFiles(ctx, startConfig.DataDir); err != nil {
			return err
		}
	}

	imagePath, err := install.resolveImagePath(ctx)
	if err != nil {
		return install.failureWithDiagnostics(
			ctx, outErr, containerName, fmt.Errorf("failed to resolve Nano image: %w", err),
		)
	}
	loadOutput, err := install.runContainerOutput(
		ctx, out, outErr, "image", "load", "--input", imagePath.RuntimePath,
	)
	if err != nil {
		return install.failureWithDiagnostics(ctx, outErr, containerName,
			fmt.Errorf("failed to load Nano image %s: %w", imagePath.RuntimePath, err))
	}
	loadedImage := strings.TrimSpace(loadOutput)
	if loadedImage == "" {
		return install.failureWithDiagnostics(ctx, outErr, containerName,
			errors.New("could not determine the image reference reported by container image load"))
	}

	if err := install.environment.Sync(ctx, out, outErr); err != nil {
		return install.failureWithDiagnostics(ctx, outErr, containerName,
			fmt.Errorf("failed to synchronize local runtime storage before starting Nano: %w", err))
	}

	bootstrap := freshDeployment && startConfig.InitialPassword != nil
	if bootstrap {
		if err := install.writeBootstrapPassword(ctx, startConfig); err != nil {
			return err
		}
	}
	args := containerRunArgs(startConfig, containerName, loadedImage, freshDeployment, bootstrap)
	var startDiagnostic bytes.Buffer
	startErrOutput := io.Writer(&startDiagnostic)
	if outErr != nil {
		startErrOutput = io.MultiWriter(outErr, &startDiagnostic)
	}
	if err := install.runContainer(ctx, out, startErrOutput, args...); err != nil {
		failure := fmt.Errorf("failed to start Nano container %s: %w", containerName, err)
		failure = classifyContainerStartBindFailure(
			ctx, install, outErr, containerName, startConfig, failure, startDiagnostic.String(),
		)
		if bootstrap {
			if removeErr := install.RemoveBootstrapPassword(ctx, startConfig.BootstrapDir); removeErr != nil {
				failure = errors.Join(failure, removeErr)
			}
		}

		return install.failureWithDiagnostics(ctx, outErr, containerName, failure)
	}

	return nil
}

func (install *AppleContainerInstall) Stop(ctx context.Context, out, outErr io.Writer) error {
	containerName, err := getContainerName(install.deployment)
	if err != nil {
		return err
	}
	exists, err := install.containerExists(ctx, outErr, containerName)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	if err := install.runContainer(ctx, out, outErr, "delete", "--force", containerName); err != nil {
		return fmt.Errorf("failed to remove Nano container %s: %w", containerName, err)
	}

	return nil
}

func (install *AppleContainerInstall) Status(
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

func (install *AppleContainerInstall) Destroy(ctx context.Context, out, outErr io.Writer) error {
	return install.Stop(ctx, out, outErr)
}

func (install *AppleContainerInstall) writeBootstrapPassword(
	ctx context.Context,
	startConfig StartConfig,
) error {
	password, err := startConfig.InitialPassword(ctx)
	if err != nil {
		return fmt.Errorf("failed to resolve the initial database password: %w", err)
	}
	if password == "" {
		return errors.New("the initial database password is empty")
	}
	path := filepath.Join(startConfig.BootstrapDir, nanoBootstrapPasswordFile)
	if err := install.environment.WriteFileAtomically(
		ctx, path, []byte(password), bootstrapDirMode, bootstrapFileMode,
	); err != nil {
		return fmt.Errorf("failed to write the initial database password file: %w", err)
	}

	return nil
}

// RemoveBootstrapPassword deletes the initial password file from
// bootstrapDir, through the execution environment rather than the host
// filesystem directly. It is safe to call when the file does not exist.
func (install *AppleContainerInstall) RemoveBootstrapPassword(
	ctx context.Context,
	bootstrapDir string,
) error {
	if strings.TrimSpace(bootstrapDir) == "" {
		return nil
	}
	path := filepath.Join(bootstrapDir, nanoBootstrapPasswordFile)
	if err := install.environment.RemoveFile(ctx, path); err != nil {
		return fmt.Errorf("failed to remove the initial database password file: %w", err)
	}

	return nil
}

func (install *AppleContainerInstall) failureWithDiagnostics(
	ctx context.Context,
	outErr io.Writer,
	containerName string,
	failure error,
) error {
	install.writeContainerDiagnostics(ctx, outErr, containerName)

	return failure
}

func (install *AppleContainerInstall) writeContainerDiagnostics(
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

	_, _ = fmt.Fprintf(diagnosticOut, "Apple container diagnostics for %s:\n", containerName)
	commands := [][]string{
		{"system", "status"},
		{"list", "--all"},
		{"inspect", containerName},
		{"logs", containerName},
	}
	for _, command := range commands {
		_, _ = fmt.Fprintf(diagnosticOut, "$ container %s\n", strings.Join(command, " "))
		if err := install.runContainer(
			diagnosticCtx, diagnosticOut, diagnosticOut, command...,
		); err != nil {
			_, _ = fmt.Fprintf(diagnosticOut, "diagnostic command failed: %v\n", err)
		}
	}
}

func (install *AppleContainerInstall) validateStartConfig(startConfig StartConfig) error {
	if startConfig.ContainerDBPort <= 0 || startConfig.ContainerDBPort > 65535 {
		return fmt.Errorf("invalid Nano published DB port %d", startConfig.ContainerDBPort)
	}
	if strings.TrimSpace(startConfig.DataDir) == "" {
		return errors.New("nano data directory is required")
	}
	if startConfig.InitialPassword != nil && strings.TrimSpace(startConfig.BootstrapDir) == "" {
		return errors.New("nano bootstrap directory is required for an initial password")
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
	if len(startConfig.SLCs) > 0 {
		return ErrScriptLanguageContainersUnsupported
	}

	return nil
}

func (install *AppleContainerInstall) isFreshDeployment(
	ctx context.Context,
	dataDir string,
) (bool, error) {
	configPath := filepath.Join(dataDir, "exasol.conf")
	exists, err := install.environment.PathExists(ctx, configPath)
	if err != nil {
		return false, fmt.Errorf("failed to inspect Nano configuration %s: %w", configPath, err)
	}

	return !exists, nil
}

func (install *AppleContainerInstall) removeStaleNanoTLSFiles(
	ctx context.Context,
	dataDir string,
) error {
	for _, relativePath := range nanoTLSFiles {
		path := filepath.Join(dataDir, relativePath)
		if err := install.environment.RemoveFile(ctx, path); err != nil {
			return fmt.Errorf("failed to remove stale Nano TLS file %s: %w", path, err)
		}
	}

	return nil
}

func (install *AppleContainerInstall) resolveImagePath(ctx context.Context) (RuntimePath, error) {
	if install.resolveImage == nil {
		return RuntimePath{}, errors.New("nano runtime artifact resolver is required")
	}
	path, err := install.resolveImage(ctx)
	if err != nil {
		return RuntimePath{}, err
	}
	if strings.TrimSpace(path.RuntimePath) == "" {
		return RuntimePath{}, errors.New("nano runtime artifact path is required")
	}

	return path, nil
}

func (install *AppleContainerInstall) containerRunning(
	ctx context.Context,
	outErr io.Writer,
	containerName string,
) (bool, error) {
	status, err := install.inspectContainerStatus(ctx, outErr, containerName)

	return status.Running, err
}

func (install *AppleContainerInstall) containerExists(
	ctx context.Context,
	outErr io.Writer,
	containerName string,
) (bool, error) {
	status, err := install.inspectContainerStatus(ctx, outErr, containerName)

	return status.Exists, err
}

// containerInspectEntry decodes only the fields `container inspect` reports
// that this install needs, out of the full OCI-ish configuration it returns.
type containerInspectEntry struct {
	Status struct {
		State string `json:"state"`
	} `json:"status"`
}

func (install *AppleContainerInstall) inspectContainerStatus(
	ctx context.Context,
	outErr io.Writer,
	containerName string,
) (podmanContainerStatus, error) {
	output, err := install.runContainerOutput(ctx, nil, outErr, "inspect", containerName)
	if err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) && exitError.ExitCode() == 1 {
			return podmanContainerStatus{}, nil
		}

		return podmanContainerStatus{}, fmt.Errorf(
			"failed to inspect Nano container %s: %w", containerName, err,
		)
	}
	var entries []containerInspectEntry
	if err := json.Unmarshal([]byte(output), &entries); err != nil || len(entries) == 0 {
		return podmanContainerStatus{}, fmt.Errorf(
			"failed to parse inspect output for Nano container %s: %w", containerName, err,
		)
	}

	return podmanContainerStatus{
		Exists:  true,
		Running: entries[0].Status.State == "running",
	}, nil
}

func (install *AppleContainerInstall) runContainer(
	ctx context.Context,
	out, outErr io.Writer,
	arg ...string,
) error {
	return install.environment.Run(
		ctx, nil, out, outErr, append([]string{"container"}, arg...)...,
	)
}

func (install *AppleContainerInstall) runContainerOutput(
	ctx context.Context,
	out, outErr io.Writer,
	arg ...string,
) (string, error) {
	var stdout bytes.Buffer
	if out == nil {
		out = &stdout
	} else {
		out = io.MultiWriter(out, &stdout)
	}
	err := install.environment.Run(
		ctx, nil, out, outErr, append([]string{"container"}, arg...)...,
	)

	return stdout.String(), err
}

func classifyContainerStartBindFailure(
	ctx context.Context,
	install *AppleContainerInstall,
	outErr io.Writer,
	containerName string,
	startConfig StartConfig,
	failure error,
	diagnostic string,
) error {
	if startConfig.ContainerDBBindHost == "" {
		return failure
	}

	status, err := install.inspectContainerStatus(ctx, outErr, containerName)
	if err != nil || status.Running {
		return failure
	}

	return localports.ClassifyBindFailure(
		"db",
		startConfig.ContainerDBPort,
		failure,
		diagnostic,
	)
}

func containerDBPortMapping(startConfig StartConfig) string {
	ports := fmt.Sprintf("%d:%d", startConfig.ContainerDBPort, nanoInternalDBPort)
	if startConfig.ContainerDBBindHost == "" {
		return ports
	}

	return startConfig.ContainerDBBindHost + ":" + ports
}

func containerRunArgs(
	startConfig StartConfig,
	containerName, loadedImage string,
	fresh, bootstrap bool,
) []string {
	args := []string{
		"run", "-d",
		"--name", containerName,
		"--shm-size=" + containerShmSize,
		"--ulimit", "nproc=unlimited",
		"-p", containerDBPortMapping(startConfig),
		"-v", startConfig.DataDir + ":" + nanoDataMountPath,
	}
	if bootstrap {
		args = append(args,
			"-v", startConfig.BootstrapDir+":"+nanoBootstrapMountPath+":ro",
		)
	}
	if startConfig.VersionCheck.Enabled {
		args = append(args, "-e", "VERSION_CHECK_IDENTITY="+startConfig.VersionCheck.Identity)
	}
	args = append(args, loadedImage, "init")
	if fresh && len(startConfig.InitParams) > 0 {
		args = append(args, "params="+strings.Join(startConfig.InitParams, " "))
	}
	if bootstrap {
		args = append(args,
			"sys_password_file="+nanoBootstrapMountPath+"/"+nanoBootstrapPasswordFile,
		)
	}
	args = append(args, nanoVersionCheckInitArgs(startConfig.VersionCheck)...)

	return args
}
