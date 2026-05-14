// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	localstate "github.com/exasol/exasol-personal/internal/localruntime/state"
	"github.com/exasol/exasol-personal/internal/localruntime/vm"
)

const (
	defaultKernelAppend        = "console=hvc0 rdinit=/init init=/init"
	defaultRestartPolicy       = "always"
	defaultGuestProvisionTag   = "exa-provision"
	defaultGuestProvisionMount = "/.exanano/provision"
	defaultGuestPayloadTag     = "exa-payload"
	defaultGuestPayloadMount   = "/.exanano/payload"
	defaultGuestLogsTag        = "exa-logs"
	defaultGuestLogsMount      = "/.exanano/logs"
	// defaultHostShareTag is the single virtio-fs tag the exasol-nano-vm
	// disk-image guest mounts at /mnt/host. The guest's existing
	// load-shared-container.sh reads its container tarball + manifest from
	// that directory.
	defaultHostShareTag       = "hostshare"
	entrypointWrapperFileName = "exasol-localruntime-entrypoint.sh"
	bootstrapProfileFileName  = "profile.sh"
	localRuntimeDirMode       = 0o700
	localRuntimeFileMode      = 0o600
	localRuntimeExecFileMode  = 0o700
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

	ports, err := r.EnsureConnectionPorts()
	if err != nil {
		return nil, err
	}
	sizing, err := r.LoadMachineSizing()
	if err != nil {
		return nil, err
	}
	state, err = r.LoadState()
	if err != nil {
		return nil, err
	}

	controller := r.Controller()
	if err := controller.Ensure(); err != nil {
		return nil, err
	}

	if state.Payload.Disk != nil {
		return r.prepareEFIDiskGuest(state, sizing, ports, controller)
	}

	return r.prepareLinuxBootGuest(state, sizing, ports, controller)
}

func (r *Runtime) prepareLinuxBootGuest(
	state *localstate.State,
	sizing *MachineSizing,
	ports *ConnectionPorts,
	controller Controller,
) (*GuestConfig, error) {
	boot, err := resolveBootAssets(state.Payload)
	if err != nil {
		return nil, err
	}

	layerDiskImage, err := r.ensureLayerDiskImage(sizing.LayerDiskBytes)
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
		Name:       deploymentMachineName(r.layout.DeploymentDir()),
		BootMode:   vm.BootModeLinux,
		KernelPath: boot.KernelPath,
		InitrdPath: boot.InitrdPath,
		KernelCommandLine: buildKernelCommandLine(
			state.Payload,
			sharedDirs,
			ports.DB,
			ports.UI,
		),
		DiskImage:             layerDiskImage,
		CPUCount:              sizing.CPUCount,
		MemoryBytes:           sizing.MemoryBytes,
		MachineIdentifierPath: r.layout.MachineIdentifierFile(),
		ConsoleLogPath:        r.layout.ConsoleLogFile(),
		SharedDirs:            sharedDirs,
	}

	return &GuestConfig{
		Controller: controller,
		Machine:    machineConfig,
	}, nil
}

// prepareEFIDiskGuest builds the guest config for booting a UEFI disk image
// produced by exasol-nano-vm. The disk image already contains Alpine + podman
// + a load-shared-container service that reads /mnt/host. We stage the
// container tarball and a JSON manifest into a single "hostshare" virtio-fs
// share. The guest's existing OpenRC service picks them up and runs the
// container.
func (r *Runtime) prepareEFIDiskGuest(
	state *localstate.State,
	sizing *MachineSizing,
	ports *ConnectionPorts,
	controller Controller,
) (*GuestConfig, error) {
	disk, err := resolveDiskAsset(state.Payload)
	if err != nil {
		return nil, err
	}

	bootDisk, err := r.stageBootDiskImage(disk.Path)
	if err != nil {
		return nil, err
	}

	hostShare, err := r.prepareHostShare(state.Payload, ports)
	if err != nil {
		return nil, err
	}

	sharedDirs := []vm.SharedDir{hostShare}

	machineConfig := vm.MachineConfig{
		Name:                  deploymentMachineName(r.layout.DeploymentDir()),
		BootMode:              vm.BootModeEFI,
		DiskImage:             bootDisk,
		EFIVariableStorePath:  r.layout.EFIVariableStoreFile(),
		CPUCount:              sizing.CPUCount,
		MemoryBytes:           sizing.MemoryBytes,
		MachineIdentifierPath: r.layout.MachineIdentifierFile(),
		ConsoleLogPath:        r.layout.ConsoleLogFile(),
		SharedDirs:            sharedDirs,
	}

	return &GuestConfig{
		Controller: controller,
		Machine:    machineConfig,
	}, nil
}

func resolveDiskAsset(payload *localstate.PayloadRef) (*localstate.PayloadDiskRef, error) {
	if payload == nil {
		return nil, ErrPayloadSelectionMissing
	}
	if payload.Disk == nil {
		return nil, fmt.Errorf("%w: payload does not describe a disk image", ErrPayloadBootAssetsMissing)
	}
	if !isCachedFile(payload.Disk.Path) {
		return nil, fmt.Errorf(
			"%w: disk image not cached at %q",
			ErrPayloadBootAssetsMissing,
			payload.Disk.Path,
		)
	}

	return payload.Disk, nil
}

// stageBootDiskImage hard-links (or copies) the cached disk image into the
// deployment's VM dir so vz reads from a stable, deployment-owned path. We
// also use the staged path as the rw target for vz, leaving the asset cache
// pristine.
func (r *Runtime) stageBootDiskImage(sourcePath string) (string, error) {
	targetPath := r.layout.BootDiskImagePath()

	if info, err := os.Stat(targetPath); err == nil && !info.IsDir() {
		return targetPath, nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("failed to stat staged boot disk image: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(targetPath), localRuntimeDirMode); err != nil {
		return "", fmt.Errorf("failed to create VM dir: %w", err)
	}

	if err := os.Link(sourcePath, targetPath); err == nil {
		return targetPath, nil
	}

	if err := copyFile(sourcePath, targetPath, localRuntimeFileMode); err != nil {
		return "", fmt.Errorf("failed to stage boot disk image: %w", err)
	}
	return targetPath, nil
}

// prepareHostShare stages a container tarball + container-manifest.json into
// the deployment's host-share dir so the guest's load-shared-container
// service finds them at /mnt/host. Refreshes the staged tarball when the
// cached source has changed (sha256 mismatch).
func (r *Runtime) prepareHostShare(
	payload *localstate.PayloadRef,
	ports *ConnectionPorts,
) (vm.SharedDir, error) {
	if err := os.MkdirAll(r.layout.HostShareDir(), localRuntimeDirMode); err != nil {
		return vm.SharedDir{}, fmt.Errorf("failed to create host share dir: %w", err)
	}

	if payload.Container != nil && strings.TrimSpace(payload.Container.Path) != "" {
		if err := r.stageContainerArchive(payload.Container); err != nil {
			return vm.SharedDir{}, err
		}
		if err := r.writeContainerManifest(payload.Container, ports); err != nil {
			return vm.SharedDir{}, err
		}
	}

	return vm.SharedDir{
		Tag:         defaultHostShareTag,
		Source:      r.layout.HostShareDir(),
		Destination: "/mnt/host",
		ReadOnly:    false,
	}, nil
}

func (r *Runtime) stageContainerArchive(container *localstate.PayloadContainerRef) error {
	target := r.layout.ContainerImageArchivePath()
	checksumPath := r.layout.ContainerImageChecksumPath()

	currentSha, err := sha256File(container.Path)
	if err != nil {
		return fmt.Errorf("failed to hash container archive: %w", err)
	}

	if existing, err := os.ReadFile(checksumPath); err == nil &&
		strings.TrimSpace(string(existing)) == currentSha {
		if _, statErr := os.Stat(target); statErr == nil {
			return nil
		}
	}

	if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to reset staged container archive: %w", err)
	}

	if err := os.Link(container.Path, target); err != nil {
		if err := copyFile(container.Path, target, localRuntimeFileMode); err != nil {
			return fmt.Errorf("failed to stage container archive: %w", err)
		}
	}

	if err := os.WriteFile(
		checksumPath,
		[]byte(currentSha+"\n"),
		localRuntimeFileMode,
	); err != nil {
		return fmt.Errorf("failed to write container archive checksum: %w", err)
	}
	return nil
}

func (r *Runtime) writeContainerManifest(
	container *localstate.PayloadContainerRef,
	_ *ConnectionPorts,
) error {
	type manifest struct {
		ContainerFile string   `json:"containerFile"`
		Ports         []int    `json:"ports"`
		Args          []string `json:"args"`
		ShmSize       string   `json:"shmSize,omitempty"`
	}

	args := container.Args
	if args == nil {
		args = []string{}
	}
	ports := container.Ports
	if len(ports) == 0 {
		// The guest container publishes on the same ports the host expects to
		// forward to. exasol-nano-vm's load-shared-container declares ports
		// for documentation/logging; the actual binding happens inside the
		// container via --network host.
		ports = []int{8563, 8443}
	}

	body, err := json.MarshalIndent(manifest{
		ContainerFile: filepath.Base(r.layout.ContainerImageArchivePath()),
		Ports:         ports,
		Args:          args,
		ShmSize:       container.ShmSize,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode container manifest: %w", err)
	}

	if err := os.WriteFile(
		r.layout.ContainerManifestPath(),
		append(body, '\n'),
		localRuntimeFileMode,
	); err != nil {
		return fmt.Errorf("failed to write container manifest: %w", err)
	}
	return nil
}

func sha256File(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
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

func (r *Runtime) ensureLayerDiskImage(sizeBytes int64) (string, error) {
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
		if err := file.Truncate(sizeBytes); err != nil {
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
	parts = append(parts, "exa_sql_port="+strconv.Itoa(dbPort))
	parts = append(parts, "exa_ui_port="+strconv.Itoa(uiPort))

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
