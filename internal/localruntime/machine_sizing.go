// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
)

const (
	defaultGuestMemoryBytes    = 4 * 1024 * 1024 * 1024
	defaultGuestLayerDiskBytes = 64 * 1024 * 1024 * 1024
	minimumGuestCPUCount       = 2
)

type MachineSizing struct {
	CPUCount       int    `json:"cpuCount,omitempty"`
	MemoryBytes    uint64 `json:"memoryBytes,omitempty"`
	LayerDiskBytes int64  `json:"layerDiskBytes,omitempty"`
}

func (r *Runtime) LoadMachineSizing() (*MachineSizing, error) {
	if err := r.EnsureRoot(); err != nil {
		return nil, err
	}

	path := r.layout.MachineSizingPath()
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			sizing := defaultMachineSizing()
			if err := writeMachineSizing(path, sizing); err != nil {
				return nil, err
			}

			return sizing, nil
		}

		return nil, fmt.Errorf("failed to read local runtime machine sizing: %w", err)
	}

	var sizing MachineSizing
	if err := json.Unmarshal(raw, &sizing); err != nil {
		return nil, fmt.Errorf("failed to decode local runtime machine sizing: %w", err)
	}

	normalized := normalizeMachineSizing(&sizing)
	if sizing != *normalized {
		if err := writeMachineSizing(path, normalized); err != nil {
			return nil, err
		}
	}

	return normalized, nil
}

func writeMachineSizing(path string, sizing *MachineSizing) error {
	data, err := json.MarshalIndent(sizing, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode local runtime machine sizing: %w", err)
	}

	if err := os.WriteFile(path, append(data, '\n'), localRuntimeFileMode); err != nil {
		return fmt.Errorf("failed to write local runtime machine sizing: %w", err)
	}

	return nil
}

func normalizeMachineSizing(sizing *MachineSizing) *MachineSizing {
	if sizing == nil {
		return defaultMachineSizing()
	}

	normalized := *sizing
	if normalized.CPUCount <= 0 {
		normalized.CPUCount = defaultGuestCPUCount()
	} else if normalized.CPUCount < minimumGuestCPUCount {
		normalized.CPUCount = minimumGuestCPUCount
	}
	if normalized.MemoryBytes == 0 {
		normalized.MemoryBytes = defaultGuestMemoryBytes
	}
	if normalized.LayerDiskBytes <= 0 {
		normalized.LayerDiskBytes = defaultGuestLayerDiskBytes
	}

	return &normalized
}

func defaultMachineSizing() *MachineSizing {
	return &MachineSizing{
		CPUCount:       defaultGuestCPUCount(),
		MemoryBytes:    defaultGuestMemoryBytes,
		LayerDiskBytes: defaultGuestLayerDiskBytes,
	}
}

func defaultGuestCPUCount() int {
	count := runtime.NumCPU()
	if count < minimumGuestCPUCount {
		return minimumGuestCPUCount
	}

	return count
}
