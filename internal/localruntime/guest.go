// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	localstate "github.com/exasol/exasol-personal/internal/localruntime/state"
	"github.com/exasol/exasol-personal/internal/localruntime/vm"
)

const (
	defaultKernelAppend        = "console=hvc0 rdinit=/init init=/init"
	defaultRestartPolicy       = "always"
	defaultJupyterEnabled      = false
	defaultJupyterPort         = 8888
	defaultVoilaPort           = 8866
	defaultGuestMemoryBytes    = 4 * 1024 * 1024 * 1024
	defaultGuestLayerDiskBytes = 64 * 1024 * 1024 * 1024
	defaultGuestProvisionTag   = "exa-provision"
	defaultGuestProvisionMount = "/.exanano/provision"
	defaultGuestPayloadTag     = "exa-payload"
	defaultGuestPayloadMount   = "/.exanano/payload"
	defaultGuestLogsTag        = "exa-logs"
	defaultGuestLogsMount      = "/.exanano/logs"
	entrypointWrapperFileName  = "exasol-localruntime-entrypoint.sh"
	bootstrapProfileFileName   = "profile.sh"
	localRuntimeDirMode        = 0o700
	localRuntimeFileMode       = 0o600
	localRuntimeExecFileMode   = 0o700
	minimumGuestCPUCount       = 2
)

var ErrPayloadSelectionMissing = errors.New("local runtime payload selection is missing")

type GuestConfig struct {
	Controller Controller
	Machine    vm.MachineConfig
}

type bootAssets struct {
	KernelPath string
	InitrdPath string
}

func (r *Runtime) PrepareGuest(ctx context.Context) (*GuestConfig, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := r.EnsureRoot(); err != nil {
		return nil, err
	}

	state, err := r.LoadState()
	if err != nil {
		return nil, err
	}
	if state.Payload == nil || strings.TrimSpace(state.Payload.CachePath) == "" {
		return nil, ErrPayloadSelectionMissing
	}

	dbPort, err := r.ensurePort("db")
	if err != nil {
		return nil, err
	}
	uiPort, err := r.ensurePort("ui")
	if err != nil {
		return nil, err
	}
	state, err = r.LoadState()
	if err != nil {
		return nil, err
	}

	boot, err := resolveBootAssets(state.Payload)
	if err != nil {
		return nil, err
	}

	controller := r.Controller()
	if err := controller.Ensure(); err != nil {
		return nil, err
	}

	layerDiskImage, err := r.ensureLayerDiskImage()
	if err != nil {
		return nil, err
	}

	payloadShare, err := r.preparePayloadShare(state.Payload)
	if err != nil {
		return nil, err
	}
	provisionShare, err := r.prepareBootstrapShare()
	if err != nil {
		return nil, err
	}

	sharedDirs := []vm.SharedDir{
		controller.SharedDir(),
		{
			Tag:         defaultGuestLogsTag,
			Source:      r.layout.LogsDir(),
			Destination: defaultGuestLogsMount,
			ReadOnly:    false,
		},
		payloadShare,
		provisionShare,
	}

	machineConfig := vm.MachineConfig{
		Name:                  deploymentMachineName(r.layout.DeploymentDir()),
		KernelPath:            boot.KernelPath,
		InitrdPath:            boot.InitrdPath,
		KernelCommandLine:     buildKernelCommandLine(state.Payload, sharedDirs, dbPort, uiPort),
		DiskImage:             layerDiskImage,
		CPUCount:              defaultGuestCPUCount(),
		MemoryBytes:           defaultGuestMemoryBytes,
		MachineIdentifierPath: r.layout.MachineIdentifierFile(),
		ConsoleLogPath:        r.layout.ConsoleLogFile(),
		SharedDirs:            sharedDirs,
	}

	return &GuestConfig{
		Controller: controller,
		Machine:    machineConfig,
	}, nil
}

func (r *Runtime) ensurePort(name string) (int, error) {
	port, err := r.AllocatePort(name)
	if err != nil {
		return 0, fmt.Errorf("failed to allocate local runtime %s port: %w", name, err)
	}

	return port, nil
}

func resolveBootAssets(payload *localstate.PayloadRef) (*bootAssets, error) {
	if payload == nil {
		return nil, ErrPayloadSelectionMissing
	}
	if payload.Boot == nil {
		return nil, ErrPayloadBootAssetsMissing
	}
	if !isCachedFile(payload.Boot.KernelPath) || !isCachedFile(payload.Boot.InitrdPath) {
		return nil, fmt.Errorf(
			"%w: kernel=%q initrd=%q",
			ErrPayloadBootAssetsMissing,
			payload.Boot.KernelPath,
			payload.Boot.InitrdPath,
		)
	}

	return &bootAssets{
		KernelPath: strings.TrimSpace(payload.Boot.KernelPath),
		InitrdPath: strings.TrimSpace(payload.Boot.InitrdPath),
	}, nil
}

func (r *Runtime) preparePayloadShare(payload *localstate.PayloadRef) (vm.SharedDir, error) {
	sourcePath, err := resolvePayloadExecutablePath(payload)
	if err != nil {
		return vm.SharedDir{}, err
	}
	if err := os.MkdirAll(r.layout.PayloadShareDir(), localRuntimeDirMode); err != nil {
		return vm.SharedDir{}, fmt.Errorf(
			"failed to create local runtime payload share dir: %w",
			err,
		)
	}

	targetPath := r.layout.PayloadExecutablePath()
	refresh, err := stagedPayloadRefreshRequired(
		targetPath,
		r.layout.PayloadChecksumPath(),
		payload,
	)
	if err != nil {
		return vm.SharedDir{}, err
	}
	if refresh {
		if err := stagePayloadExecutable(sourcePath, targetPath); err != nil {
			return vm.SharedDir{}, err
		}
		if err := writePayloadChecksum(
			r.layout.PayloadChecksumPath(),
			payload.Checksum,
		); err != nil {
			return vm.SharedDir{}, err
		}
	}

	return vm.SharedDir{
		Tag:         defaultGuestPayloadTag,
		Source:      r.layout.PayloadShareDir(),
		Destination: defaultGuestPayloadMount,
		ReadOnly:    false,
	}, nil
}

func resolvePayloadExecutablePath(payload *localstate.PayloadRef) (string, error) {
	if payload == nil || strings.TrimSpace(payload.CachePath) == "" {
		return "", fmt.Errorf(
			"%w: selected payload cache path is empty",
			ErrPayloadSelectionMissing,
		)
	}

	sourcePath := strings.TrimSpace(payload.CachePath)
	info, err := os.Stat(sourcePath)
	if err != nil {
		return "", fmt.Errorf("failed to stat selected local runtime payload: %w", err)
	}
	if !info.IsDir() {
		return sourcePath, nil
	}

	candidates, err := filepath.Glob(filepath.Join(sourcePath, "*.run"))
	if err != nil {
		return "", fmt.Errorf("failed to inspect local runtime payload dir: %w", err)
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf(
			"local runtime payload dir %q does not contain a .run artifact",
			sourcePath,
		)
	}

	return candidates[0], nil
}

func stagedPayloadRefreshRequired(
	targetPath string,
	checksumPath string,
	payload *localstate.PayloadRef,
) (bool, error) {
	if !isCachedFile(targetPath) {
		return true, nil
	}
	expectedChecksum := strings.TrimSpace(payload.Checksum)
	if expectedChecksum == "" {
		return false, nil
	}

	data, err := os.ReadFile(checksumPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return true, nil
		}

		return false, fmt.Errorf("failed to read staged payload checksum: %w", err)
	}

	return strings.TrimSpace(string(data)) != expectedChecksum, nil
}

func stagePayloadExecutable(sourcePath string, targetPath string) error {
	if err := os.Remove(targetPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to reset staged local runtime payload: %w", err)
	}

	if err := os.Link(sourcePath, targetPath); err != nil {
		if err := copyFile(sourcePath, targetPath, localRuntimeExecFileMode); err != nil {
			return fmt.Errorf("failed to stage local runtime payload: %w", err)
		}

		return nil
	}

	//nolint:gosec // Staged payloads must remain executable inside the guest VM.
	if err := os.Chmod(targetPath, localRuntimeExecFileMode); err != nil {
		return fmt.Errorf("failed to mark staged payload executable: %w", err)
	}

	return nil
}

func copyFile(sourcePath string, targetPath string, mode os.FileMode) error {
	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	targetFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}

	if _, err := io.Copy(targetFile, sourceFile); err != nil {
		_ = targetFile.Close()
		return err
	}

	return targetFile.Close()
}

func writePayloadChecksum(path string, checksum string) error {
	if err := os.WriteFile(
		path,
		[]byte(strings.TrimSpace(checksum)+"\n"),
		localRuntimeFileMode,
	); err != nil {
		return fmt.Errorf("failed to write staged payload checksum: %w", err)
	}

	return nil
}

func (r *Runtime) ensureLayerDiskImage() (string, error) {
	diskImagePath := r.layout.LayerDiskImageFile()
	file, err := os.OpenFile(diskImagePath, os.O_CREATE|os.O_RDWR, localRuntimeFileMode)
	if err != nil {
		return "", fmt.Errorf("failed to open local runtime layer disk image: %w", err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("failed to stat local runtime layer disk image: %w", err)
	}
	if info.Size() == 0 {
		if err := file.Truncate(defaultGuestLayerDiskBytes); err != nil {
			return "", fmt.Errorf("failed to size local runtime layer disk image: %w", err)
		}
	}

	return diskImagePath, nil
}

func (r *Runtime) prepareBootstrapShare() (vm.SharedDir, error) {
	if err := os.MkdirAll(r.layout.BootstrapDir(), localRuntimeDirMode); err != nil {
		return vm.SharedDir{}, fmt.Errorf("failed to create local runtime bootstrap dir: %w", err)
	}

	files := map[string]struct {
		content []byte
		mode    os.FileMode
	}{
		bootstrapProfileFileName: {
			content: guestBootstrapProfile,
			mode:    localRuntimeFileMode,
		},
		entrypointWrapperFileName: {
			content: guestEntrypointWrapper,
			mode:    localRuntimeExecFileMode,
		},
	}
	for name, file := range files {
		path := filepath.Join(r.layout.BootstrapDir(), name)
		//nolint:gosec // The guest entrypoint wrapper must remain executable inside the VM.
		if err := os.WriteFile(
			path,
			append(file.content, '\n'),
			file.mode,
		); err != nil {
			return vm.SharedDir{}, fmt.Errorf(
				"failed to write local runtime bootstrap asset %q: %w",
				name,
				err,
			)
		}
	}

	return vm.SharedDir{
		Tag:         defaultGuestProvisionTag,
		Source:      r.layout.BootstrapDir(),
		Destination: defaultGuestProvisionMount,
		ReadOnly:    false,
	}, nil
}

func buildKernelCommandLine(
	payload *localstate.PayloadRef,
	sharedDirs []vm.SharedDir,
	dbPort int,
	uiPort int,
) string {
	parts := []string{
		defaultKernelAppend,
		"exa_restart=" + defaultRestartPolicy,
	}

	for _, sharedDir := range sharedDirs {
		parts = append(parts, "exa_volume="+sharedDir.Tag+":"+sharedDir.Destination)
	}

	parts = append(parts, "exa_layer_key="+bootstrapLayerKey(payload))
	parts = append(parts, "exa_udf_ccache=/overlay-storage/.exanano/.ccache")
	parts = append(parts, "exa_sql_port="+strconv.Itoa(dbPort))
	parts = append(parts, "exa_ui_port="+strconv.Itoa(uiPort))
	parts = append(parts, "exa_jupyter_enabled="+strconv.FormatBool(defaultJupyterEnabled))
	parts = append(parts, "exa_jupyter_port="+strconv.Itoa(defaultJupyterPort))
	parts = append(parts, "exa_voila_port="+strconv.Itoa(defaultVoilaPort))

	return strings.Join(parts, " ")
}

func bootstrapLayerKey(payload *localstate.PayloadRef) string {
	hash := sha256.New()

	for _, value := range []string{
		payloadValue(payload, func(ref *localstate.PayloadRef) string { return ref.Version }),
		payloadValue(payload, func(ref *localstate.PayloadRef) string { return ref.Architecture }),
		payloadValue(payload, func(ref *localstate.PayloadRef) string { return ref.Checksum }),
		string(guestBootstrapProfile),
		string(guestEntrypointWrapper),
	} {
		_, _ = hash.Write([]byte(value))
		_, _ = hash.Write([]byte{0})
	}

	return hex.EncodeToString(hash.Sum(nil))
}

func payloadValue(
	payload *localstate.PayloadRef,
	selector func(*localstate.PayloadRef) string,
) string {
	if payload == nil {
		return ""
	}

	return selector(payload)
}

func deploymentMachineName(deploymentDir string) string {
	name := filepath.Base(strings.TrimSpace(deploymentDir))
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == string(filepath.Separator) {
		return "local-exasol"
	}

	return name
}

func defaultGuestCPUCount() int {
	count := runtime.NumCPU()
	if count <= 0 {
		return minimumGuestCPUCount
	}
	if count < minimumGuestCPUCount {
		return minimumGuestCPUCount
	}

	return count
}
