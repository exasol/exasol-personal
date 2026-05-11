// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package assets

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var (
	ErrEmbeddedPayloadUnavailable = errors.New("embedded local runtime payload is unavailable")
	ErrEmbeddedPayloadInvalid     = errors.New("embedded local runtime payload is invalid")
)

const embeddedBundleCacheDirName = "bundle"

type EmbeddedMetadata struct {
	Version      string      `json:"version"`
	Architecture string      `json:"architecture"`
	Run          *Asset      `json:"run,omitempty"`
	Boot         *BootAssets `json:"boot,omitempty"`
}

type EmbeddedPayload struct {
	Metadata *EmbeddedMetadata
	Bundle   []byte
}

type SeededPayload struct {
	Version      string
	Architecture string
	RunChecksum  string
	RunPath      string
	Boot         *CachedBootAssets
}

func LoadEmbeddedPayload(
	metadataJSON []byte,
	bundle []byte,
	expectedArchitecture string,
) (*EmbeddedPayload, error) {
	if len(strings.TrimSpace(string(metadataJSON))) == 0 || len(bundle) == 0 {
		return nil, ErrEmbeddedPayloadUnavailable
	}

	var metadata EmbeddedMetadata
	if err := json.Unmarshal(metadataJSON, &metadata); err != nil {
		return nil, fmt.Errorf("failed to decode embedded payload metadata: %w", err)
	}

	if strings.TrimSpace(metadata.Version) == "" {
		return nil, fmt.Errorf("%w: embedded payload version is missing", ErrEmbeddedPayloadInvalid)
	}
	if strings.TrimSpace(metadata.Architecture) == "" {
		return nil, fmt.Errorf("%w: embedded payload architecture is missing", ErrEmbeddedPayloadInvalid)
	}
	if strings.TrimSpace(expectedArchitecture) != "" &&
		strings.TrimSpace(metadata.Architecture) != strings.TrimSpace(expectedArchitecture) {
		return nil, fmt.Errorf(
			"%w: expected architecture %q but embedded payload is %q",
			ErrEmbeddedPayloadInvalid,
			expectedArchitecture,
			metadata.Architecture,
		)
	}
	if metadata.Run == nil || strings.TrimSpace(metadata.Run.Filename) == "" ||
		strings.TrimSpace(metadata.Run.SHA256) == "" {
		return nil, fmt.Errorf("%w: embedded run payload metadata is incomplete", ErrEmbeddedPayloadInvalid)
	}
	if metadata.Boot == nil || metadata.Boot.Kernel == nil || metadata.Boot.Initrd == nil {
		return nil, fmt.Errorf("%w: embedded boot asset metadata is incomplete", ErrEmbeddedPayloadInvalid)
	}
	if strings.TrimSpace(metadata.Boot.Kernel.Filename) == "" ||
		strings.TrimSpace(metadata.Boot.Kernel.SHA256) == "" {
		return nil, fmt.Errorf("%w: embedded kernel metadata is incomplete", ErrEmbeddedPayloadInvalid)
	}
	if strings.TrimSpace(metadata.Boot.Initrd.Filename) == "" ||
		strings.TrimSpace(metadata.Boot.Initrd.SHA256) == "" {
		return nil, fmt.Errorf("%w: embedded initrd metadata is incomplete", ErrEmbeddedPayloadInvalid)
	}

	return &EmbeddedPayload{
		Metadata: &metadata,
		Bundle:   bundle,
	}, nil
}

func SeedEmbeddedPayload(cacheDir string, payload *EmbeddedPayload) (*SeededPayload, error) {
	if payload == nil || payload.Metadata == nil {
		return nil, fmt.Errorf("%w: nil embedded payload", ErrEmbeddedPayloadUnavailable)
	}

	destinationRoot := filepath.Join(
		cacheDir,
		strings.TrimSpace(payload.Metadata.Version),
		strings.TrimSpace(payload.Metadata.Architecture),
		embeddedBundleCacheDirName,
	)

	bundle, err := verifySeededBundle(destinationRoot, payload.Metadata)
	if err != nil {
		if err := os.RemoveAll(destinationRoot); err != nil {
			return nil, fmt.Errorf("failed to reset embedded payload cache dir: %w", err)
		}

		bundle, err = PrepareBundleBytes(payload.Bundle, destinationRoot)
		if err != nil {
			return nil, err
		}
	}

	return buildSeededPayload(bundle, payload.Metadata)
}

func verifySeededBundle(destinationRoot string, metadata *EmbeddedMetadata) (*Bundle, error) {
	bundle, err := discoverBundle(destinationRoot)
	if err != nil {
		return nil, err
	}

	if _, err := buildSeededPayload(bundle, metadata); err != nil {
		return nil, err
	}

	return bundle, nil
}

func buildSeededPayload(bundle *Bundle, metadata *EmbeddedMetadata) (*SeededPayload, error) {
	if bundle == nil || metadata == nil {
		return nil, fmt.Errorf("%w: missing embedded bundle metadata", ErrEmbeddedPayloadInvalid)
	}

	runPath, err := verifyBundleFile(bundle.RunPath, metadata.Run)
	if err != nil {
		return nil, err
	}
	kernelPath, err := verifyBundleFile(bundle.KernelPath, metadata.Boot.Kernel)
	if err != nil {
		return nil, err
	}
	initrdPath, err := verifyBundleFile(bundle.InitrdPath, metadata.Boot.Initrd)
	if err != nil {
		return nil, err
	}

	return &SeededPayload{
		Version:      strings.TrimSpace(metadata.Version),
		Architecture: strings.TrimSpace(metadata.Architecture),
		RunChecksum:  strings.TrimSpace(metadata.Run.SHA256),
		RunPath:      runPath,
		Boot: &CachedBootAssets{
			KernelPath: kernelPath,
			InitrdPath: initrdPath,
		},
	}, nil
}

func verifyBundleFile(path string, asset *Asset) (string, error) {
	if asset == nil {
		return "", fmt.Errorf("%w: missing bundle asset metadata", ErrEmbeddedPayloadInvalid)
	}
	if strings.TrimSpace(asset.Filename) != "" && filepath.Base(path) != strings.TrimSpace(asset.Filename) {
		return "", fmt.Errorf(
			"%w: expected bundle file %q, got %q",
			ErrEmbeddedPayloadInvalid,
			asset.Filename,
			filepath.Base(path),
		)
	}

	ok, err := verifyFileSHA256(path, asset.SHA256)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf(
			"%w: expected sha256 %s for %s",
			ErrPayloadVerificationFailed,
			asset.SHA256,
			filepath.Base(path),
		)
	}

	return path, nil
}
