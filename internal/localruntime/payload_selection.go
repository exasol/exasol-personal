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
	if payload.Boot == nil || payload.Boot.Kernel == nil || payload.Boot.Initrd == nil {
		return nil, fmt.Errorf(
			"%w: payload %s/%s does not describe kernel and initrd assets",
			ErrPayloadBootAssetsMissing,
			strings.TrimSpace(payload.Version),
			strings.TrimSpace(payload.Architecture),
		)
	}

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

func cachedPayloadRef(state *localstate.State) *localstate.PayloadRef {
	if state == nil || state.Payload == nil {
		return nil
	}
	if !isCachedFile(state.Payload.CachePath) {
		return nil
	}
	if state.Payload.Boot == nil {
		return nil
	}
	if !isCachedFile(state.Payload.Boot.KernelPath) ||
		!isCachedFile(state.Payload.Boot.InitrdPath) {
		return nil
	}

	return state.Payload
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
