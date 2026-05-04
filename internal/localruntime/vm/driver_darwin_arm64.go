// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

//go:build darwin && arm64

package vm

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Code-Hex/vz/v3"
)

type driver struct {
	mu          sync.RWMutex
	machine     *vz.VirtualMachine
	state       vz.VirtualMachineState
	done        chan struct{}
	terminalErr error
}

func New() Driver {
	return &driver{}
}

func (d *driver) Start(ctx context.Context, config MachineConfig) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := validateMachineConfig(config); err != nil {
		return err
	}

	d.mu.Lock()
	if d.machine != nil && !isTerminalState(d.state) {
		d.mu.Unlock()
		return ErrAlreadyRunning
	}
	d.mu.Unlock()

	vmConfig, err := buildVirtualMachineConfiguration(config)
	if err != nil {
		return err
	}

	machine, err := vz.NewVirtualMachine(vmConfig)
	if err != nil {
		return fmt.Errorf("failed to create virtual machine: %w", err)
	}

	done := make(chan struct{})
	d.mu.Lock()
	d.machine = machine
	d.state = machine.State()
	d.done = done
	d.terminalErr = nil
	d.mu.Unlock()

	if err := machine.Start(); err != nil {
		startErr := fmt.Errorf("failed to start virtual machine: %w", err)
		d.finish(machine, done, vz.VirtualMachineStateError, startErr)
		return startErr
	}

	currentState := machine.State()
	d.mu.Lock()
	if d.machine == machine {
		d.state = currentState
	}
	d.mu.Unlock()

	if isTerminalState(currentState) {
		terminalErr := error(nil)
		if currentState == vz.VirtualMachineStateError {
			terminalErr = fmt.Errorf("virtual machine entered %s state", currentState.String())
		}
		d.finish(machine, done, currentState, terminalErr)
		return nil
	}

	go d.watchLifecycle(machine, done)

	return nil
}

func (d *driver) Stop(ctx context.Context) error {
	machine, done := d.snapshot()
	if machine == nil {
		return nil
	}

	if machine.CanRequestStop() {
		requested, err := machine.RequestStop()
		if err == nil && requested {
			return d.waitForDone(ctx, done)
		}
		if err != nil && !machine.CanStop() {
			return fmt.Errorf("failed to request virtual machine stop: %w", err)
		}
	}

	if !machine.CanStop() {
		return errors.New("virtual machine cannot be stopped in its current state")
	}

	if err := machine.Stop(); err != nil {
		return fmt.Errorf("failed to force-stop virtual machine: %w", err)
	}

	return d.waitForDone(ctx, done)
}

func (d *driver) Wait(ctx context.Context) error {
	_, done := d.snapshot()
	if done == nil {
		return nil
	}

	return d.waitForDone(ctx, done)
}

func (d *driver) Running() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.machine != nil && !isTerminalState(d.state)
}

func (d *driver) snapshot() (*vz.VirtualMachine, chan struct{}) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.machine, d.done
}

func (d *driver) waitForDone(ctx context.Context, done chan struct{}) error {
	select {
	case <-done:
		d.mu.RLock()
		defer d.mu.RUnlock()
		return d.terminalErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (d *driver) watchLifecycle(machine *vz.VirtualMachine, done chan struct{}) {
	for state := range machine.StateChangedNotify() {
		if !d.recordState(machine, done, state) {
			return
		}
	}
}

func (d *driver) recordState(
	machine *vz.VirtualMachine,
	done chan struct{},
	state vz.VirtualMachineState,
) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.machine != machine {
		return false
	}

	d.state = state
	if isTerminalState(state) {
		if state == vz.VirtualMachineStateError && d.terminalErr == nil {
			d.terminalErr = fmt.Errorf("virtual machine entered %s state", state.String())
		}
		d.machine = nil
		close(done)
		return false
	}

	return true
}

func (d *driver) finish(
	machine *vz.VirtualMachine,
	done chan struct{},
	state vz.VirtualMachineState,
	err error,
) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.machine != machine {
		return
	}

	d.state = state
	d.terminalErr = err
	d.machine = nil
	close(done)
}

func isTerminalState(state vz.VirtualMachineState) bool {
	return state == vz.VirtualMachineStateStopped || state == vz.VirtualMachineStateError
}

func buildVirtualMachineConfiguration(config MachineConfig) (*vz.VirtualMachineConfiguration, error) {
	bootLoader, err := buildBootLoader(config)
	if err != nil {
		return nil, err
	}

	vmConfig, err := vz.NewVirtualMachineConfiguration(
		bootLoader,
		clampCPUCount(
			config.CPUCount,
			vz.VirtualMachineConfigurationMinimumAllowedCPUCount(),
			vz.VirtualMachineConfigurationMaximumAllowedCPUCount(),
		),
		clampMemoryBytes(
			config.MemoryBytes,
			vz.VirtualMachineConfigurationMinimumAllowedMemorySize(),
			vz.VirtualMachineConfigurationMaximumAllowedMemorySize(),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create virtual machine configuration: %w", err)
	}

	if platformConfig, err := buildPlatformConfiguration(config.MachineIdentifierPath); err != nil {
		return nil, err
	} else if platformConfig != nil {
		vmConfig.SetPlatformVirtualMachineConfiguration(platformConfig)
	}

	serialPorts, err := buildSerialPorts(config.ConsoleLogPath)
	if err != nil {
		return nil, err
	}
	if len(serialPorts) > 0 {
		vmConfig.SetSerialPortsVirtualMachineConfiguration(serialPorts)
	}

	networkDevices, err := buildNetworkDevices(config.MACAddress)
	if err != nil {
		return nil, err
	}
	vmConfig.SetNetworkDevicesVirtualMachineConfiguration(networkDevices)

	storageDevices, err := buildStorageDevices(config.DiskImagePath)
	if err != nil {
		return nil, err
	}
	if len(storageDevices) > 0 {
		vmConfig.SetStorageDevicesVirtualMachineConfiguration(storageDevices)
	}

	directoryShares, err := buildDirectoryShares(config.SharedDirs)
	if err != nil {
		return nil, err
	}
	if len(directoryShares) > 0 {
		vmConfig.SetDirectorySharingDevicesVirtualMachineConfiguration(directoryShares)
	}

	entropyDevice, err := vz.NewVirtioEntropyDeviceConfiguration()
	if err != nil {
		return nil, fmt.Errorf("failed to create entropy device configuration: %w", err)
	}
	vmConfig.SetEntropyDevicesVirtualMachineConfiguration([]*vz.VirtioEntropyDeviceConfiguration{entropyDevice})

	valid, err := vmConfig.Validate()
	if err != nil {
		return nil, fmt.Errorf("invalid virtual machine configuration: %w", err)
	}
	if !valid {
		return nil, errors.New("invalid virtual machine configuration")
	}

	return vmConfig, nil
}

func buildBootLoader(config MachineConfig) (*vz.EFIBootLoader, error) {
	efiVarsPath := strings.TrimSpace(config.EFIVarsPath)
	if efiVarsPath == "" {
		return nil, errors.New("machine EFI variable store path is required")
	}

	if err := os.MkdirAll(filepath.Dir(efiVarsPath), 0o700); err != nil {
		return nil, fmt.Errorf("failed to create EFI variable store dir: %w", err)
	}

	variableStoreOpts := []vz.NewEFIVariableStoreOption{}
	if _, err := os.Stat(efiVarsPath); errors.Is(err, os.ErrNotExist) {
		variableStoreOpts = append(variableStoreOpts, vz.WithCreatingEFIVariableStore())
	} else if err != nil {
		return nil, fmt.Errorf("failed to stat EFI variable store: %w", err)
	}

	variableStore, err := vz.NewEFIVariableStore(efiVarsPath, variableStoreOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create EFI variable store: %w", err)
	}

	bootLoader, err := vz.NewEFIBootLoader(vz.WithEFIVariableStore(variableStore))
	if err != nil {
		return nil, fmt.Errorf("failed to create EFI boot loader: %w", err)
	}

	return bootLoader, nil
}

func buildPlatformConfiguration(machineIdentifierPath string) (*vz.GenericPlatformConfiguration, error) {
	if strings.TrimSpace(machineIdentifierPath) == "" {
		return nil, nil
	}

	machineIdentifier, err := loadOrCreateMachineIdentifier(machineIdentifierPath)
	if err != nil {
		return nil, err
	}

	platformConfig, err := vz.NewGenericPlatformConfiguration(
		vz.WithGenericMachineIdentifier(machineIdentifier),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create generic platform configuration: %w", err)
	}

	return platformConfig, nil
}

func loadOrCreateMachineIdentifier(path string) (*vz.GenericMachineIdentifier, error) {
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if len(data) == 0 {
			return nil, fmt.Errorf("machine identifier file %q is empty", path)
		}

		machineIdentifier, loadErr := vz.NewGenericMachineIdentifierWithData(data)
		if loadErr != nil {
			return nil, fmt.Errorf("failed to load machine identifier: %w", loadErr)
		}

		return machineIdentifier, nil
	case errors.Is(err, os.ErrNotExist):
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, fmt.Errorf("failed to create machine identifier dir: %w", err)
		}

		machineIdentifier, createErr := vz.NewGenericMachineIdentifier()
		if createErr != nil {
			return nil, fmt.Errorf("failed to create machine identifier: %w", createErr)
		}

		if err := os.WriteFile(path, machineIdentifier.DataRepresentation(), 0o600); err != nil {
			return nil, fmt.Errorf("failed to persist machine identifier: %w", err)
		}

		return machineIdentifier, nil
	default:
		return nil, fmt.Errorf("failed to read machine identifier file: %w", err)
	}
}

func buildSerialPorts(consoleLogPath string) ([]*vz.VirtioConsoleDeviceSerialPortConfiguration, error) {
	if strings.TrimSpace(consoleLogPath) == "" {
		return nil, nil
	}

	if err := os.MkdirAll(filepath.Dir(consoleLogPath), 0o700); err != nil {
		return nil, fmt.Errorf("failed to create console log dir: %w", err)
	}
	file, err := os.OpenFile(consoleLogPath, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare console log file: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("failed to prepare console log file: %w", err)
	}

	attachment, err := vz.NewFileSerialPortAttachment(consoleLogPath, true)
	if err != nil {
		return nil, fmt.Errorf("failed to create console log attachment: %w", err)
	}

	serialPort, err := vz.NewVirtioConsoleDeviceSerialPortConfiguration(attachment)
	if err != nil {
		return nil, fmt.Errorf("failed to create console device serial port: %w", err)
	}

	return []*vz.VirtioConsoleDeviceSerialPortConfiguration{serialPort}, nil
}

func buildNetworkDevices(requestedMAC string) ([]*vz.VirtioNetworkDeviceConfiguration, error) {
	attachment, err := vz.NewNATNetworkDeviceAttachment()
	if err != nil {
		return nil, fmt.Errorf("failed to create NAT network attachment: %w", err)
	}

	networkDevice, err := vz.NewVirtioNetworkDeviceConfiguration(attachment)
	if err != nil {
		return nil, fmt.Errorf("failed to create virtio network device configuration: %w", err)
	}

	macAddress, err := buildMACAddress(requestedMAC)
	if err != nil {
		return nil, err
	}
	networkDevice.SetMACAddress(macAddress)

	return []*vz.VirtioNetworkDeviceConfiguration{networkDevice}, nil
}

func buildMACAddress(requested string) (*vz.MACAddress, error) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		mac, err := vz.NewRandomLocallyAdministeredMACAddress()
		if err != nil {
			return nil, fmt.Errorf("failed to create MAC address: %w", err)
		}

		return mac, nil
	}

	parsed, err := net.ParseMAC(requested)
	if err != nil {
		return nil, fmt.Errorf("failed to parse requested MAC %q: %w", requested, err)
	}
	mac, err := vz.NewMACAddress(parsed)
	if err != nil {
		return nil, fmt.Errorf("failed to create MAC address from %q: %w", requested, err)
	}

	return mac, nil
}

func buildStorageDevices(diskImagePath string) ([]vz.StorageDeviceConfiguration, error) {
	if strings.TrimSpace(diskImagePath) == "" {
		return nil, nil
	}

	attachment, err := vz.NewDiskImageStorageDeviceAttachment(strings.TrimSpace(diskImagePath), false)
	if err != nil {
		return nil, fmt.Errorf("failed to create disk image attachment: %w", err)
	}

	storageDevice, err := vz.NewVirtioBlockDeviceConfiguration(attachment)
	if err != nil {
		return nil, fmt.Errorf("failed to create virtio block device configuration: %w", err)
	}

	return []vz.StorageDeviceConfiguration{storageDevice}, nil
}

func buildDirectoryShares(sharedDirs []SharedDir) ([]vz.DirectorySharingDeviceConfiguration, error) {
	if len(sharedDirs) == 0 {
		return nil, nil
	}

	directoryShares := make([]vz.DirectorySharingDeviceConfiguration, 0, len(sharedDirs))
	for index, sharedDir := range sharedDirs {
		hostDir := strings.TrimSpace(sharedDir.Source)
		tag := resolvedSharedDirTag(sharedDir, index)

		directory, err := vz.NewSharedDirectory(hostDir, sharedDir.ReadOnly)
		if err != nil {
			return nil, fmt.Errorf("failed to create shared directory %q: %w", hostDir, err)
		}

		share, err := vz.NewSingleDirectoryShare(directory)
		if err != nil {
			return nil, fmt.Errorf("failed to create directory share %q: %w", hostDir, err)
		}

		fileSystemDevice, err := vz.NewVirtioFileSystemDeviceConfiguration(tag)
		if err != nil {
			return nil, fmt.Errorf("failed to create filesystem device %q: %w", tag, err)
		}
		fileSystemDevice.SetDirectoryShare(share)

		directoryShares = append(directoryShares, fileSystemDevice)
	}

	return directoryShares, nil
}
