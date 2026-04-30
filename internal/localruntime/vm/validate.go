// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package vm

import (
	"errors"
	"fmt"
	"strings"
)

func validateMachineConfig(config MachineConfig) error {
	if strings.TrimSpace(config.Name) == "" {
		return errors.New("machine name is required")
	}
	if strings.TrimSpace(config.DiskImagePath) == "" {
		return errors.New("machine disk image path is required")
	}
	if strings.TrimSpace(config.EFIVarsPath) == "" {
		return errors.New("machine EFI variable store path is required")
	}
	if len(config.PortForwards) > 0 {
		return fmt.Errorf(
			"%w: use a host-side forwarder in the local runtime layer",
			ErrPortForwardUnsupported,
		)
	}

	for index, sharedDir := range config.SharedDirs {
		if strings.TrimSpace(sharedDir.Source) == "" {
			return fmt.Errorf("shared directory %d source path is required", index+1)
		}
		if resolvedSharedDirTag(sharedDir, index) == "" {
			return fmt.Errorf("shared directory %d resolved to an empty tag", index+1)
		}
	}

	return nil
}
