// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"

	localassets "github.com/exasol/exasol-personal/internal/localruntime/assets"
	localstate "github.com/exasol/exasol-personal/internal/localruntime/state"
)

const (
	PayloadMetadataURLEnvVar  = "EXASOL_LOCAL_RUNTIME_PAYLOAD_METADATA_URL"
	DefaultPayloadMetadataURL = "https://personal-exanano-artifacts-" +
		"686382907660-eu-central-1-an.s3.eu-central-1.amazonaws.com/" +
		"localruntime/metadata.json"
)

var ErrPayloadBootAssetsMissing = errors.New("local runtime payload boot assets are missing")

type payloadManager interface {
	Resolve(ctx context.Context, architecture string) (*localassets.Payload, error)
	EnsureCached(ctx context.Context, payload *localassets.Payload) (string, error)
	EnsureBootCached(
		ctx context.Context,
		payload *localassets.Payload,
	) (*localassets.CachedBootAssets, error)
	EnsureDiskCached(
		ctx context.Context,
		payload *localassets.Payload,
	) (*localassets.CachedDiskAsset, error)
	EnsureContainerCached(
		ctx context.Context,
		payload *localassets.Payload,
	) (*localassets.CachedContainerAsset, error)
}

var (
	defaultPayloadCacheDir = localassets.DefaultCacheDir
	newPayloadManager      = func(metadataURL string, cacheDir string) payloadManager {
		return localassets.NewManager(metadataURL, cacheDir)
	}
	resolvePayloadMetadataURL = GetPayloadMetadataURL
)

func GetPayloadMetadataURL() string {
	if value := strings.TrimSpace(os.Getenv(PayloadMetadataURLEnvVar)); value != "" {
		return value
	}

	return DefaultPayloadMetadataURL
}

func (r *Runtime) EnsurePayloadSelected(ctx context.Context) (*localstate.PayloadRef, error) {
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
	if cachedPayloadRef(state) != nil {
		return state.Payload, nil
	}

	cacheDir, err := defaultPayloadCacheDir()
	if err != nil {
		return nil, err
	}

	manager := newPayloadManager(resolvePayloadMetadataURL(), cacheDir)
	payload, err := manager.Resolve(ctx, localPayloadArchitecture())
	if err != nil {
		return nil, err
	}

	switch {
	case payload.Disk != nil:
		return r.persistDiskPayload(ctx, manager, payload, state)
	case payload.Boot != nil && payload.Boot.Kernel != nil && payload.Boot.Initrd != nil:
		return r.persistLinuxPayload(ctx, manager, payload, state)
	default:
		return nil, fmt.Errorf(
			"%w: payload %s/%s describes neither a disk image nor kernel+initrd assets",
			ErrPayloadBootAssetsMissing,
			strings.TrimSpace(payload.Version),
			strings.TrimSpace(payload.Architecture),
		)
	}
}

func (r *Runtime) persistLinuxPayload(
	ctx context.Context,
	manager payloadManager,
	payload *localassets.Payload,
	state *localstate.State,
) (*localstate.PayloadRef, error) {
	cachePath, err := manager.EnsureCached(ctx, payload)
	if err != nil {
		return nil, err
	}
	bootAssets, err := manager.EnsureBootCached(ctx, payload)
	if err != nil {
		return nil, err
	}

	state.Payload = &localstate.PayloadRef{
		Version:      strings.TrimSpace(payload.Version),
		Architecture: strings.TrimSpace(payload.Architecture),
		Checksum:     strings.TrimSpace(payload.SHA256),
		CachePath:    cachePath,
		Boot: &localstate.PayloadBootRef{
			KernelPath: strings.TrimSpace(bootAssets.KernelPath),
			InitrdPath: strings.TrimSpace(bootAssets.InitrdPath),
		},
	}
	if err := r.SaveState(state); err != nil {
		return nil, err
	}

	return state.Payload, nil
}

func (r *Runtime) persistDiskPayload(
	ctx context.Context,
	manager payloadManager,
	payload *localassets.Payload,
	state *localstate.State,
) (*localstate.PayloadRef, error) {
	diskAsset, err := manager.EnsureDiskCached(ctx, payload)
	if err != nil {
		return nil, err
	}

	ref := &localstate.PayloadRef{
		Version:      strings.TrimSpace(payload.Version),
		Architecture: strings.TrimSpace(payload.Architecture),
		Checksum:     strings.TrimSpace(payload.Disk.SHA256),
		CachePath:    diskAsset.Path,
		Disk:         &localstate.PayloadDiskRef{Path: diskAsset.Path},
	}

	if payload.Container != nil {
		containerAsset, err := manager.EnsureContainerCached(ctx, payload)
		if err != nil {
			return nil, err
		}
		if containerAsset != nil {
			ref.Container = &localstate.PayloadContainerRef{
				Path:    containerAsset.Path,
				ShmSize: strings.TrimSpace(payload.Container.ShmSize),
				Ports:   payload.Container.Ports,
				Args:    payload.Container.Args,
			}
		}
	}

	state.Payload = ref
	if err := r.SaveState(state); err != nil {
		return nil, err
	}

	return state.Payload, nil
}

func cachedPayloadRef(state *localstate.State) *localstate.PayloadRef {
	if state == nil || state.Payload == nil {
		return nil
	}
	if !isCachedFile(state.Payload.CachePath) {
		return nil
	}

	switch {
	case state.Payload.Disk != nil:
		if !isCachedFile(state.Payload.Disk.Path) {
			return nil
		}
		if state.Payload.Container != nil && !isCachedFile(state.Payload.Container.Path) {
			return nil
		}
		return state.Payload
	case state.Payload.Boot != nil:
		if !isCachedFile(state.Payload.Boot.KernelPath) ||
			!isCachedFile(state.Payload.Boot.InitrdPath) {
			return nil
		}
		return state.Payload
	default:
		return nil
	}
}

func isCachedFile(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}

	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}

	return true
}

func localPayloadArchitecture() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "arm64"
	default:
		return runtime.GOARCH
	}
}
