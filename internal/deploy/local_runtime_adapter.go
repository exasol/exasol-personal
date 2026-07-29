// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/exasol/exasol-personal/assets/localruntimebin"
	"github.com/exasol/exasol-personal/assets/localworkloadbin"
	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/runtimeadapter"
	"github.com/exasol/exasol-personal/internal/util"
)

const (
	nanoVersionCheckInterval    = 86400
	localRuntimeDirMode         = 0o750
	localProviderExecutableMode = 0o700
	localPortEntryFields        = 2
)

type localPortSelectionPolicy int

const (
	localPortMayFallBack localPortSelectionPolicy = iota
	localPortMustBeExact
)

type preparedLocalRuntime struct {
	adapter runtimeadapter.RuntimeAdapter
	spec    runtimeadapter.WorkloadSpec
}

var (
	readLocalWorkloadMetadata = localworkloadbin.ReadMetadata
	loadLocalWorkloadArchive  = localworkloadbin.Archive
)

func prepareLocalRuntimeAdapter(
	ctx context.Context,
	deployment config.DeploymentDir,
	runtimeConfig localRuntimeConfig,
) (*preparedLocalRuntime, error) {
	launcherState, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		return nil, err
	}

	requestedPort, userOverride, err := parseLocalDatabasePort(runtimeConfig.ports)
	if err != nil {
		return nil, err
	}
	portPolicy := localPortMayFallBack
	if userOverride {
		portPolicy = localPortMustBeExact
	}
	preferredPort := preferredLocalDatabasePort(deployment, requestedPort, userOverride)
	resolvedPort, err := resolveLocalDatabasePort(
		ctx,
		preferredPort,
		portPolicy,
	)
	if err != nil {
		return nil, err
	}
	binaryPath := localVMBinaryPath(deployment)
	if runtime.GOOS == localMacOS {
		binaryPath, err = stageLocalVMBinary(deployment)
		if err != nil {
			return nil, err
		}
	}

	prepared, err := buildLocalRuntimeAdapter(
		deployment,
		runtimeConfig,
		launcherState,
		resolvedPort,
		binaryPath,
	)
	if err != nil {
		return nil, err
	}

	return prepared, nil
}

// reconstructLocalRuntimeAdapter derives a runtime plan without changing the
// deployment. It is safe for status, diagnostics, and shared-lock operations.
func reconstructLocalRuntimeAdapter(
	deployment config.DeploymentDir,
	runtimeConfig localRuntimeConfig,
) (*preparedLocalRuntime, error) {
	launcherState, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		return nil, err
	}
	requestedPort, userOverride, err := parseLocalDatabasePort(runtimeConfig.ports)
	if err != nil {
		return nil, err
	}
	resolvedPort := preferredLocalDatabasePort(deployment, requestedPort, userOverride)

	return buildLocalRuntimeAdapter(
		deployment,
		runtimeConfig,
		launcherState,
		resolvedPort,
		localVMBinaryPath(deployment),
	)
}

func buildLocalRuntimeAdapter(
	deployment config.DeploymentDir,
	runtimeConfig localRuntimeConfig,
	launcherState *config.ExasolPersonalState,
	databasePort int,
	localVMBinary string,
) (*preparedLocalRuntime, error) {
	metadata, err := readLocalWorkloadMetadata()
	if err != nil {
		return nil, err
	}
	versionCheck := deriveRuntimeVersionCheckSettings(deployment, launcherState)
	slcMounts := make([]runtimeadapter.SLCMount, 0, len(launcherState.InstalledSLCs))
	for _, installed := range launcherState.InstalledSLCs {
		slcMounts = append(slcMounts, runtimeadapter.SLCMount{
			Image:  installed.Image,
			Target: installed.Target,
		})
	}
	dataPath := filepath.Join(deployment.Root(), "local", "data", "exa")
	spec := runtimeadapter.WorkloadSpec{
		DeploymentID:     launcherState.DeploymentId,
		ImageReference:   metadata.ImageReference,
		ImageDigest:      metadata.ImageDigest,
		LoadImageArchive: loadLocalWorkloadArchive,
		DataPath:         dataPath,
		DBHostAddress:    "127.0.0.1",
		DBHostPort:       databasePort,
		CPUs:             runtimeConfig.cpuCount,
		MemoryMiB:        runtimeConfig.memoryMB,
		NanoArguments:    []string{"params=maxConnectionsLicenseLimit=20"},
		VersionCheck:     versionCheck,
		SLCMounts:        slcMounts,
		Security:         runtimeadapter.DefaultContainerSecurity(),
	}

	var adapter runtimeadapter.RuntimeAdapter
	platform := runtime.GOOS
	if os.Getenv(localAllowUnsupportedEnv) != "" &&
		platform != localMacOS &&
		platform != localWindowsOS {
		platform = localMacOS
	}
	switch platform {
	case localMacOS:
		macAdapter := runtimeadapter.NewMacVMAdapter(
			deployment.Root(),
			localVMBinary,
			runtimeadapter.OSCommandRunner{},
		)
		adapter = macAdapter
	case localWindowsOS:
		adapter = runtimeadapter.NewWindowsPodmanAdapter(
			deployment.Root(),
			runtimeadapter.OSCommandRunner{},
		)
	default:
		adapter = runtimeadapter.NewLinuxPodmanAdapter(
			deployment.Root(),
			runtimeadapter.OSCommandRunner{},
		)
	}

	return &preparedLocalRuntime{
		adapter: adapter,
		spec:    spec,
	}, nil
}

func preferredLocalDatabasePort(
	deployment config.DeploymentDir,
	requestedPort int,
	userOverride bool,
) int {
	if userOverride {
		return requestedPort
	}
	info, err := config.ReadDeploymentInfo(deployment)
	if err == nil && info.Connection != nil && info.Connection.DBPort > 0 {
		return info.Connection.DBPort
	}

	return requestedPort
}

func deriveRuntimeVersionCheckSettings(
	deployment config.DeploymentDir,
	state *config.ExasolPersonalState,
) runtimeadapter.VersionCheckSettings {
	settings := runtimeadapter.VersionCheckSettings{Enabled: state.VersionCheckEnabled}
	if !settings.Enabled {
		return settings
	}
	details := GetVersionCheckDetails(deployment)
	settings.URL = details.URL
	settings.Identity = state.ClusterIdentity
	settings.OperatingSystem = details.OperatingSystem
	settings.IntervalSeconds = nanoVersionCheckInterval
	settings.RetryIntervalSeconds = nanoVersionCheckInterval

	return settings
}

func stageLocalVMBinary(deployment config.DeploymentDir) (string, error) {
	path := localVMBinaryPath(deployment)
	if current, err := os.ReadFile(path); err == nil &&
		bytes.Equal(current, localruntimebin.RunnerBinary) {
		//nolint:gosec // The staged local-vm provider must be executable.
		if err := os.Chmod(path, localProviderExecutableMode); err != nil {
			return "", err
		}

		return path, nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), localRuntimeDirMode); err != nil {
		return "", err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".local-vm-*")
	if err != nil {
		return "", err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Close(); err != nil {
		return "", err
	}
	if err := localruntimebin.WriteBinary(temporaryPath); err != nil {
		return "", err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return "", err
	}

	return path, nil
}

func localVMBinaryPath(deployment config.DeploymentDir) string {
	return filepath.Join(deployment.Root(), "local", "provider", "local-vm")
}

func parseLocalDatabasePort(raw string) (int, bool, error) {
	if strings.TrimSpace(raw) == "" {
		return runtimeadapter.NanoContainerPort, false, nil
	}
	var port int
	found := false
	for _, rawEntry := range strings.Split(raw, ",") {
		entry := strings.TrimSpace(rawEntry)
		if entry == "" {
			continue
		}
		parts := strings.SplitN(entry, ":", localPortEntryFields)
		if len(parts) != localPortEntryFields {
			return 0, false, fmt.Errorf(
				"invalid local port override %q: expected <service>:<port>",
				entry,
			)
		}
		service := strings.ToLower(strings.TrimSpace(parts[0]))
		if service != "db" && service != "database" {
			return 0, false, fmt.Errorf(
				"unsupported local port service %q; only db is configurable",
				service,
			)
		}
		if found {
			return 0, false, errors.New("database port is configured more than once")
		}
		parsed, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil || parsed < 1 || parsed > 65535 {
			return 0, false, fmt.Errorf(
				"invalid database port in override %q: expected 1-65535",
				entry,
			)
		}
		port = parsed
		found = true
	}
	if !found {
		return runtimeadapter.NanoContainerPort, false, nil
	}

	return port, true, nil
}

func resolveLocalDatabasePort(
	ctx context.Context,
	requested int,
	policy localPortSelectionPolicy,
) (int, error) {
	var listenConfig net.ListenConfig

	return resolveLocalDatabasePortUsing(
		ctx,
		requested,
		policy,
		runtime.GOOS,
		func(ctx context.Context, address string) (net.Listener, error) {
			return listenConfig.Listen(ctx, "tcp", address)
		},
	)
}

func resolveLocalDatabasePortUsing(
	ctx context.Context,
	requested int,
	policy localPortSelectionPolicy,
	platform string,
	listen func(context.Context, string) (net.Listener, error),
) (int, error) {
	if requested <= 0 || requested > 65535 {
		return 0, fmt.Errorf("database port must be between 1 and 65535: %d", requested)
	}
	listener, err := listen(
		ctx,
		net.JoinHostPort(localDeploymentPublicHost, strconv.Itoa(requested)),
	)
	if err == nil {
		if closeErr := listener.Close(); closeErr != nil {
			return 0, fmt.Errorf("failed to release database port probe: %w", closeErr)
		}

		return requested, nil
	}
	if policy == localPortMustBeExact {
		return 0, fmt.Errorf("requested database port %d is unavailable: %w", requested, err)
	}
	if platform == localMacOS {
		return 0, nil
	}

	listener, err = listen(ctx, net.JoinHostPort(localDeploymentPublicHost, "0"))
	if err != nil {
		return 0, fmt.Errorf("failed to select database port: %w", err)
	}
	defer listener.Close()
	address, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf(
			"failed to select database port: unexpected address %T",
			listener.Addr(),
		)
	}

	return address.Port, nil
}

func runtimePrerequisiteOptions(out io.Writer) runtimeadapter.PrerequisiteOptions {
	interactive := util.IsInteractiveStdin()
	if out == nil {
		out = os.Stderr
	}

	return runtimeadapter.PrerequisiteOptions{
		Interactive: interactive,
		Confirm: func(prompt string) (bool, error) {
			if !interactive {
				return false, nil
			}
			if _, err := fmt.Fprintf(out, "%s [y/N]: ", prompt); err != nil {
				return false, err
			}
			scanner := bufio.NewScanner(os.Stdin)
			if !scanner.Scan() {
				return false, scanner.Err()
			}
			answer := strings.ToLower(strings.TrimSpace(scanner.Text()))

			return answer == "y" || answer == "yes", nil
		},
	}
}

func writeRuntimeAdapterArtifacts(
	deployment config.DeploymentDir,
	status *runtimeadapter.RuntimeStatus,
	capabilities runtimeadapter.RuntimeCapabilities,
) error {
	if status == nil || status.Database.Port <= 0 {
		return errors.New("local runtime database endpoint is missing")
	}
	launcherState, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		return err
	}
	connection := &config.DeploymentConnection{
		Host:                       localDeploymentPublicHost,
		DisplayHost:                localDeploymentPublicHost,
		PublicIp:                   localDeploymentPublicHost,
		DBPort:                     status.Database.Port,
		Username:                   localDBUser,
		InsecureSkipCertValidation: true,
		ShellSupported:             capabilities.ContainerShell,
	}
	if status.VM != nil && status.VM.SSH != nil {
		connection.SSHPort = strconv.Itoa(status.VM.SSH.Port)
		if status.VM.PrivateKeyPath != "" {
			relative, err := filepath.Rel(deployment.Root(), status.VM.PrivateKeyPath)
			if err != nil {
				return err
			}
			connection.SSHCommand = fmt.Sprintf(
				"ssh -i %s %s@%s -p %s",
				filepath.ToSlash(relative),
				localSSHUser,
				localDeploymentPublicHost,
				connection.SSHPort,
			)
		}
	}
	info := &config.DeploymentInfo{
		Backend:         localDeploymentBackend,
		DeploymentId:    launcherState.DeploymentId,
		DeploymentState: StatusRunning,
		ClusterSize:     1,
		ClusterState:    StatusRunning,
		InstanceType:    "exasol-local",
		Connection:      connection,
	}
	if err := config.WriteDeploymentInfo(deployment.Root(), info); err != nil {
		return err
	}

	return config.WriteSecrets(deployment.Root(), &config.Secrets{DbPassword: localDBPassword})
}

func waitForRuntimeDatabase(
	ctx context.Context,
	deployment config.DeploymentDir,
	waitTimeoutSeconds int,
) error {
	if waitTimeoutSeconds <= 0 {
		waitTimeoutSeconds = LocalDatabaseStartedDefaultTimeoutSeconds
	}
	waitCtx, cancel := context.WithTimeout(ctx, time.Duration(waitTimeoutSeconds)*time.Second)
	defer cancel()

	return WaitForLocalDatabaseStarted(waitCtx, deployment)
}
