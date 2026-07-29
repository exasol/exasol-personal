// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

// Package runtimeadapter owns the runtime-only local workload model and its
// platform adapters. None of the types in this package are persisted.
package runtimeadapter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
)

const (
	NanoContainerPort = 8563
	DefaultShmMiB     = 512
	UnlimitedPIDs     = -1
)

// WorkloadSpec is reconstructed from Personal's deployment manifest and
// standard deployment state for every command.
type WorkloadSpec struct {
	DeploymentID     string
	ImageReference   string
	ImageDigest      string
	LoadImageArchive func() ([]byte, error)
	DataPath         string
	DBHostAddress    string
	DBHostPort       int
	CPUs             int
	MemoryMiB        int
	NanoArguments    []string
	VersionCheck     VersionCheckSettings
	SLCMounts        []SLCMount
	Security         ContainerSecurity
}

type VersionCheckSettings struct {
	Enabled              bool
	URL                  string
	Identity             string
	OperatingSystem      string
	IntervalSeconds      int
	RetryIntervalSeconds int
}

type SLCMount struct {
	Image  string
	Target string
}

type ContainerSecurity struct {
	ShmMiB    int
	PIDsLimit int
	UnmaskAll bool
}

func DefaultContainerSecurity() ContainerSecurity {
	return ContainerSecurity{
		ShmMiB:    DefaultShmMiB,
		PIDsLimit: UnlimitedPIDs,
		UnmaskAll: true,
	}
}

func (spec WorkloadSpec) Validate() error {
	if strings.TrimSpace(spec.DeploymentID) == "" {
		return errors.New("workload deployment identity is missing")
	}
	if !strings.Contains(spec.ImageReference, "@sha256:") {
		return errors.New("nano image reference must be pinned by sha256 digest")
	}
	if !strings.HasPrefix(spec.ImageDigest, "sha256:") {
		return errors.New("nano image digest must use the sha256 prefix")
	}
	if !strings.HasSuffix(spec.ImageReference, "@"+spec.ImageDigest) {
		return errors.New("nano image reference and digest do not match")
	}
	digest := strings.TrimPrefix(spec.ImageDigest, "sha256:")
	if len(digest) != sha256.Size*2 {
		return errors.New("nano image digest must contain exactly 64 hexadecimal characters")
	}
	if _, err := hex.DecodeString(digest); err != nil {
		return errors.New("nano image digest contains non-hexadecimal characters")
	}
	if strings.TrimSpace(spec.DataPath) == "" {
		return errors.New("workload data path is missing")
	}
	if spec.DBHostPort <= 0 || spec.DBHostPort > 65535 {
		return fmt.Errorf("database host port must be between 1 and 65535: %d", spec.DBHostPort)
	}
	if spec.Security.ShmMiB != DefaultShmMiB {
		return fmt.Errorf("nano requires exactly %d MiB of shared memory", DefaultShmMiB)
	}
	if spec.Security.PIDsLimit != UnlimitedPIDs {
		return errors.New("nano requires an unlimited PID limit")
	}
	if !spec.Security.UnmaskAll {
		return errors.New("nano requires an unmasked proc filesystem")
	}
	for index, mount := range spec.SLCMounts {
		if strings.TrimSpace(mount.Image) == "" {
			return fmt.Errorf("SLC mount %d has no image", index)
		}
		cleanTarget := path.Clean(mount.Target)
		if cleanTarget != mount.Target || !strings.HasPrefix(cleanTarget, "/exa/slc/") {
			return fmt.Errorf("SLC mount %d target %q must be below /exa/slc", index, mount.Target)
		}
	}

	return nil
}

func (spec WorkloadSpec) ImageBytes() ([]byte, error) {
	if spec.LoadImageArchive == nil {
		return nil, errors.New("embedded Nano image archive is unavailable")
	}
	archive, err := spec.LoadImageArchive()
	if err != nil {
		return nil, err
	}
	if len(archive) == 0 {
		return nil, errors.New("embedded Nano image archive is empty")
	}

	return archive, nil
}

type RuntimePhase string

const (
	RuntimePhaseUnknown  RuntimePhase = "unknown"
	RuntimePhaseStopped  RuntimePhase = "stopped"
	RuntimePhaseStarting RuntimePhase = "starting"
	RuntimePhaseRunning  RuntimePhase = "running"
	RuntimePhaseDegraded RuntimePhase = "degraded"
)

type RuntimeEndpoint struct {
	Address string `json:"address"`
	Port    int    `json:"port"`
	Health  string `json:"health,omitempty"`
}

type VMDetails struct {
	Phase          string                     `json:"phase"`
	PID            int                        `json:"pid,omitempty"`
	GuestIP        string                     `json:"guestIp,omitempty"`
	SSH            *RuntimeEndpoint           `json:"ssh,omitempty"`
	PrivateKeyPath string                     `json:"privateKeyPath,omitempty"`
	Forwards       map[string]RuntimeEndpoint `json:"forwards,omitempty"`
	Hook           string                     `json:"hook,omitempty"`
}

type RuntimeStatus struct {
	Phase        RuntimePhase    `json:"phase"`
	WorkloadName string          `json:"workloadName"`
	Database     RuntimeEndpoint `json:"database"`
	ContainerID  string          `json:"containerId,omitempty"`
	Message      string          `json:"message,omitempty"`
	VM           *VMDetails      `json:"vm,omitempty"`
}

type RuntimeCapabilities struct {
	SLC            bool `json:"slc"`
	VMShell        bool `json:"vmShell"`
	ContainerShell bool `json:"containerShell"`
	Resources      bool `json:"resources"`
}

func PlatformCapabilities(goos string) RuntimeCapabilities {
	if goos == "darwin" {
		return RuntimeCapabilities{
			SLC:            true,
			VMShell:        true,
			ContainerShell: true,
			Resources:      true,
		}
	}
	if goos == "windows" {
		return RuntimeCapabilities{SLC: true}
	}
	if goos == "linux" {
		return RuntimeCapabilities{SLC: true}
	}

	return RuntimeCapabilities{}
}

type PrerequisiteOptions struct {
	Interactive bool
	Confirm     func(prompt string) (bool, error)
}

type ShellKind string

const (
	ShellVM        ShellKind = "vm"
	ShellContainer ShellKind = "container"
)

// RuntimeAdapter reconciles a runtime from a fresh WorkloadSpec. Implementations
// must discover live state rather than depend on serialized adapter metadata.
type RuntimeAdapter interface {
	Prerequisites(ctx context.Context, options PrerequisiteOptions) error
	Start(
		ctx context.Context,
		spec WorkloadSpec,
		stdout, stderr io.Writer,
	) (*RuntimeStatus, error)
	Stop(ctx context.Context, spec WorkloadSpec, stdout, stderr io.Writer) error
	Status(ctx context.Context, spec WorkloadSpec) (*RuntimeStatus, error)
	Health(ctx context.Context, spec WorkloadSpec) (*RuntimeStatus, error)
	Logs(ctx context.Context, spec WorkloadSpec, stdout, stderr io.Writer) error
	Destroy(ctx context.Context, spec WorkloadSpec, stdout, stderr io.Writer) error
	Shell(
		ctx context.Context,
		spec WorkloadSpec,
		kind ShellKind,
		stdin io.Reader,
		stdout, stderr io.Writer,
	) error
	Capabilities() RuntimeCapabilities
}
