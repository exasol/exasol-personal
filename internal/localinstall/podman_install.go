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
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localports"
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
	// `unmask=ALL` and `label=disable` are required for UDFs to work, which need
	// nested namespaces (at least user, mount and pid).
	nanoSecurityOpt        = "unmask=ALL"
	nanoSELinuxSecurityOpt = "label=disable"
	// nanoBootstrapMountPath is where the first container sees the bootstrap
	// directory holding the initial sys password.
	nanoBootstrapMountPath    = "/run/secrets"
	nanoBootstrapPasswordFile = "sys_password"
	bootstrapDirMode          = 0o700
	bootstrapFileMode         = 0o600
	// nanoDataMountPath is where Nano expects its persistent data inside the
	// container; the target adds SELinux relabelling for the bind mount.
	nanoDataMountPath        = "/exa"
	nanoDataMountTarget      = nanoDataMountPath + ":Z"
	podmanDiagnosticsTimeout = 10 * time.Second
	nanoVersionCheckDefault  = 86400
	nanoVersionCheckMin      = 60
	nanoVersionCheckMax      = 604800
	nanoVersionCheckRetryMax = 86400
)

type PodmanInstall struct {
	deployment    config.DeploymentDir
	environment   ExecutionEnvironment
	client        containerClient
	resolveImage  func(context.Context) (RuntimePath, error)
	slcStagingDir RuntimePath
	slcStatusPath RuntimePath
}

func NewPodmanInstall(
	deployment config.DeploymentDir,
	manager *runtimeartifacts.Manager,
	runtimeExec []string,
	slcStagingDir string,
	slcStatusPath string,
) *PodmanInstall {
	directEnvironment := NewDirectExecutionEnvironment(runtimeExec)
	resolveImage := func(ctx context.Context) (RuntimePath, error) {
		path, err := ResolveNanoImage(ctx, manager)
		if err != nil {
			return RuntimePath{}, err
		}

		return IdentityRuntimePath(path), nil
	}

	return NewPodmanInstallWithEnvironment(
		deployment,
		directEnvironment,
		resolveImage,
		IdentityRuntimePath(slcStagingDir),
		IdentityRuntimePath(slcStatusPath),
	)
}

// ResolveNanoImage materializes the platform-specific image archive on the host.
// Runtimes that execute Podman elsewhere can stage and map this host path before
// passing it back to PodmanInstall.
func ResolveNanoImage(
	ctx context.Context,
	manager *runtimeartifacts.Manager,
) (string, error) {
	if override := strings.TrimSpace(os.Getenv(nanoImageOverridePathEnv)); override != "" {
		return override, nil
	}
	if manager == nil {
		return "", errors.New("nano runtime artifact manager is required")
	}

	return manager.Request(ctx, exasolNanoImageResourceID)
}

func NewPodmanInstallWithEnvironment(
	deployment config.DeploymentDir,
	environment ExecutionEnvironment,
	resolveImage func(context.Context) (RuntimePath, error),
	slcStagingDir RuntimePath,
	slcStatusPath RuntimePath,
) *PodmanInstall {
	return &PodmanInstall{
		deployment:    deployment,
		environment:   environment,
		client:        newContainerClient(environment),
		resolveImage:  resolveImage,
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
	// An interrupted first initialization can leave its password file behind.
	if err := install.RemoveBootstrapPassword(ctx, startConfig.BootstrapDir); err != nil {
		return err
	}
	if len(startConfig.LegacyContainerNames) > 0 {
		if err := install.adoptLegacyContainerName(
			ctx, out, outErr, containerName, startConfig.LegacyContainerNames,
		); err != nil {
			return err
		}
	}
	containerStatus, err := install.inspectContainerStatus(ctx, outErr, containerName)
	if err != nil {
		return err
	}
	migrated, err := install.migrateOverlayDataIfNeeded(
		ctx,
		out,
		outErr,
		containerName,
		startConfig.DataDir,
		containerStatus,
	)
	if err != nil {
		return err
	}
	if migrated {
		containerStatus = podmanContainerStatus{}
	}
	recovered, err := install.recoverIncompleteInitialCreate(
		ctx, out, outErr, containerName, startConfig.DataDir, containerStatus,
	)
	if err != nil {
		return err
	}
	if recovered {
		containerStatus = podmanContainerStatus{}
	}
	if containerStatus.Running {
		return nil
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
	loadedImage, err := install.client.LoadImage(
		ctx, imagePath.RuntimePath, out, outErr,
	)
	if err != nil {
		return install.failureWithDiagnostics(ctx, outErr, containerName,
			fmt.Errorf("failed to load Nano image %s: %w", imagePath.RuntimePath, err))
	}
	availableSLCs, err := install.materializeSLCs(ctx, out, outErr, containerName, startConfig.SLCs)
	if err != nil {
		return err
	}
	install.pruneUnreferencedSLCImages(ctx, outErr, startConfig.SLCs)
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
	spec := nanoRunSpec(startConfig, nanoRunOptions{
		containerName: containerName,
		loadedImage:   loadedImage,
		slcs:          availableSLCs,
		fresh:         freshDeployment,
		bootstrap:     bootstrap,
	})
	var startDiagnostic bytes.Buffer
	startErrOutput := io.Writer(&startDiagnostic)
	if outErr != nil {
		startErrOutput = io.MultiWriter(outErr, &startDiagnostic)
	}
	if err := install.client.Create(ctx, spec, out, startErrOutput); err != nil {
		failure := fmt.Errorf("failed to start Nano container %s: %w", containerName, err)
		failure = classifyPodmanStartBindFailure(
			ctx,
			install,
			outErr,
			containerName,
			startConfig,
			failure,
			startDiagnostic.String(),
		)
		if bootstrap {
			if err := install.RemoveBootstrapPassword(ctx, startConfig.BootstrapDir); err != nil {
				failure = errors.Join(failure, err)
			}
		}

		return install.failureWithDiagnostics(ctx, outErr, containerName,
			failure)
	}

	return nil
}

// RemoveHostBootstrapPassword empties a bootstrap directory on the host
// filesystem without needing the runtime, including temporary files left by an
// interrupted write. The directory stays because a container may mount it.
func RemoveHostBootstrapPassword(hostBootstrapDir string) error {
	entries, err := os.ReadDir(hostBootstrapDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to inspect the initial database password directory: %w", err)
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(hostBootstrapDir, entry.Name())); err != nil {
			return fmt.Errorf("failed to remove initial database password material: %w", err)
		}
	}

	return nil
}

// RemoveBootstrapPassword deletes the initial password file from bootstrapDir.
// It is safe to call when the file or directory does not exist.
func (install *PodmanInstall) RemoveBootstrapPassword(
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

func classifyPodmanStartBindFailure(
	ctx context.Context,
	install *PodmanInstall,
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

func nanoDBPortMapping(startConfig StartConfig) PortMapping {
	return PortMapping{
		HostIP:        startConfig.ContainerDBBindHost,
		HostPort:      startConfig.ContainerDBPort,
		ContainerPort: nanoInternalDBPort,
	}
}

type nanoRunOptions struct {
	containerName string
	loadedImage   string
	slcs          []SLCConfig
	fresh         bool
	bootstrap     bool
}

func nanoRunSpec(startConfig StartConfig, options nanoRunOptions) ContainerSpec {
	restartPolicy := RestartAlways
	if options.bootstrap {
		// Nano rejects initial-password options once its runtime is initialized,
		// and a restart replays the container's original arguments, so the
		// container that receives the initial password must not restart.
		restartPolicy = RestartNever
	}
	spec := ContainerSpec{
		Name:             options.containerName,
		Image:            options.loadedImage,
		Replace:          true,
		Restart:          restartPolicy,
		SharedMemorySize: nanoShmSize,
		ProcessLimit:     nanoPIDsLimit,
		SecurityOptions:  []string{nanoSecurityOpt, nanoSELinuxSecurityOpt},
		Ports:            []PortMapping{nanoDBPortMapping(startConfig)},
		Mounts: []Mount{
			{Kind: MountBind, Source: startConfig.DataDir, Target: nanoDataMountTarget},
		},
	}
	if options.bootstrap {
		spec.Mounts = append(spec.Mounts, Mount{
			Kind: MountBind, Source: startConfig.BootstrapDir,
			Target: nanoBootstrapMountPath, ReadOnly: true, Relabel: true,
		})
	}
	for _, slc := range options.slcs {
		// Writable works around containers that ship no directory skeleton: the UDF
		// sandbox launcher creates the mount points it binds into the container root
		// (/proc, /dev, /buckets, /var/tmp and siblings) only where that root is
		// writable, and a missing one fails every UDF as an opaque `VM crashed`.
		// The runtime keeps the writes in an ephemeral per-container overlay, so the
		// image is never modified and the UDF-visible root stays read-only.
		spec.Mounts = append(spec.Mounts, Mount{
			Kind: MountImage, Source: slc.Image, Target: slc.Target, Writable: true,
		})
	}
	if startConfig.VersionCheck.Enabled {
		spec.Env = map[string]string{
			"VERSION_CHECK_IDENTITY": startConfig.VersionCheck.Identity,
		}
	}
	spec.Args = []string{"init"}
	if options.fresh && len(startConfig.InitParams) > 0 {
		spec.Args = append(spec.Args, "params="+strings.Join(startConfig.InitParams, " "))
	}
	if options.bootstrap {
		spec.Args = append(
			spec.Args,
			"sys_password_file="+nanoBootstrapMountPath+"/"+nanoBootstrapPasswordFile,
		)
	}
	spec.Args = append(spec.Args, nanoVersionCheckInitArgs(startConfig.VersionCheck)...)

	return spec
}

func (install *PodmanInstall) Stop(ctx context.Context, out, outErr io.Writer) error {
	containerName, err := getContainerName(install.deployment)
	if err != nil {
		return err
	}
	if err := install.client.Remove(ctx, containerName, true, out, outErr); err != nil {
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

func (install *PodmanInstall) writeBootstrapPassword(
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

func (install *PodmanInstall) adoptLegacyContainerName(
	ctx context.Context,
	out, outErr io.Writer,
	containerName string,
	legacyNames []string,
) error {
	exists, err := install.containerExists(ctx, outErr, containerName)
	if err != nil || exists {
		return err
	}
	for _, legacyName := range legacyNames {
		legacyName = strings.TrimSpace(legacyName)
		if legacyName == "" || legacyName == containerName {
			continue
		}
		exists, err := install.containerExists(ctx, outErr, legacyName)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		if err := install.client.Rename(
			ctx, legacyName, containerName, out, outErr,
		); err != nil {
			return install.failureWithDiagnostics(
				ctx,
				outErr,
				legacyName,
				fmt.Errorf(
					"failed to adopt legacy Nano container %s as %s: %w",
					legacyName,
					containerName,
					err,
				),
			)
		}

		return nil
	}

	return nil
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

	_, _ = fmt.Fprintf(diagnosticOut, "Container diagnostics for %s:\n", containerName)
	install.client.Diagnostics(diagnosticCtx, containerName, diagnosticOut)
}

func (install *PodmanInstall) validateStartConfig(startConfig StartConfig) error {
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
	for index, slc := range startConfig.SLCs {
		if strings.TrimSpace(slc.Image) == "" {
			return fmt.Errorf("nano SLC %d image is required", index)
		}
		if strings.TrimSpace(slc.Target) == "" {
			return fmt.Errorf("nano SLC %d target is required", index)
		}
	}
	if startConfig.SLCs != nil {
		if strings.TrimSpace(install.slcStatusPath.RuntimePath) == "" {
			return errors.New("SLC status path is required for SLC-aware startup")
		}
		if strings.TrimSpace(install.slcStagingDir.RuntimePath) == "" {
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

func (install *PodmanInstall) isFreshDeployment(
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
	status, err := install.inspectContainerStatus(ctx, outErr, containerName)
	return status.Running, err
}

type podmanContainerStatus struct {
	Exists  bool
	Running bool
}

func (install *PodmanInstall) containerExists(
	ctx context.Context,
	outErr io.Writer,
	containerName string,
) (bool, error) {
	status, err := install.inspectContainerStatus(ctx, outErr, containerName)

	return status.Exists, err
}

// inspectContainerStatus asks podman's listing once rather than testing for
// the container and then inspecting it.
func (install *PodmanInstall) inspectContainerStatus(
	ctx context.Context,
	outErr io.Writer,
	containerName string,
) (podmanContainerStatus, error) {
	containers, err := install.client.containers(ctx, outErr, "name=^"+containerName+"$")
	if err != nil {
		return podmanContainerStatus{}, fmt.Errorf(
			"failed to find Nano container %s: %w", containerName, err,
		)
	}
	for _, container := range containers {
		if slices.Contains(container.Names, containerName) {
			return podmanContainerStatus{Exists: true, Running: container.Running()}, nil
		}
	}

	return podmanContainerStatus{}, nil
}

func (install *PodmanInstall) resolveImagePath(ctx context.Context) (RuntimePath, error) {
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

func ContainerName(deployment config.DeploymentDir) (string, error) {
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

func getContainerName(deployment config.DeploymentDir) (string, error) {
	return ContainerName(deployment)
}
