// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"context"
	"errors"
	"os"
	"runtime"
	"strings"

	embeddedpayload "github.com/exasol/exasol-personal/assets/localruntimebin"
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

var (
	defaultPayloadCacheDir = localassets.DefaultCacheDir
	resolveEmbeddedPayload = func(architecture string) (*localassets.EmbeddedPayload, error) {
		return localassets.LoadEmbeddedPayload(
			embeddedpayload.PayloadMetadata,
			embeddedpayload.PayloadBundle,
			architecture,
		)
	}
	seedEmbeddedPayload = localassets.SeedEmbeddedPayload
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

	payload, err := resolveEmbeddedPayload(localPayloadArchitecture())
	if err != nil {
		return nil, err
	}
	seeded, err := seedEmbeddedPayload(cacheDir, payload)
	if err != nil {
		return nil, err
	}

	state.Payload = &localstate.PayloadRef{
		Version:      seeded.Version,
		Architecture: seeded.Architecture,
		Checksum:     seeded.RunChecksum,
		CachePath:    seeded.RunPath,
		Boot: &localstate.PayloadBootRef{
			KernelPath: strings.TrimSpace(seeded.Boot.KernelPath),
			InitrdPath: strings.TrimSpace(seeded.Boot.InitrdPath),
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
