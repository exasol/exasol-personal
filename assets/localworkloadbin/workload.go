// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

// Package localworkloadbin exposes the platform-specific Nano image embedded
// in a Personal artifact.

//go:generate go run ../../tools/localworkload generate

package localworkloadbin

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"strings"
)

type Metadata struct {
	ImageReference string `json:"imageReference"`
	ImageDigest    string `json:"imageDigest"`
	ArchiveSHA256  string `json:"archiveSha256"`
	Platform       string `json:"platform"`
}

func ReadMetadata() (Metadata, error) {
	if !Available || len(metadataJSON) == 0 {
		return Metadata{}, errors.New("embedded Nano workload metadata is not available")
	}
	var metadata Metadata
	if err := json.Unmarshal(metadataJSON, &metadata); err != nil {
		return Metadata{}, fmt.Errorf("invalid embedded Nano image metadata: %w", err)
	}
	if !strings.HasPrefix(metadata.ImageDigest, "sha256:") ||
		!strings.HasSuffix(metadata.ImageReference, "@"+metadata.ImageDigest) {
		return Metadata{}, errors.New("embedded Nano image is not pinned by its sha256 digest")
	}
	digest := strings.TrimPrefix(metadata.ImageDigest, "sha256:")
	if len(digest) != 64 || strings.Trim(digest, "0") == "" {
		return Metadata{}, errors.New("embedded Nano image has an invalid or placeholder digest")
	}
	if _, err := hex.DecodeString(digest); err != nil {
		return Metadata{}, errors.New("embedded Nano image digest is not hexadecimal")
	}
	archiveDigest := strings.TrimPrefix(metadata.ArchiveSHA256, "sha256:")
	if len(archiveDigest) != sha256.Size*2 {
		return Metadata{}, errors.New("embedded Nano archive checksum is invalid")
	}
	if _, err := hex.DecodeString(archiveDigest); err != nil {
		return Metadata{}, errors.New("embedded Nano archive checksum is not hexadecimal")
	}
	expectedPlatform := runtime.GOOS + "/" + runtime.GOARCH
	if metadata.Platform != expectedPlatform {
		return Metadata{}, fmt.Errorf(
			"embedded Nano image targets %s, expected %s",
			metadata.Platform,
			expectedPlatform,
		)
	}

	return metadata, nil
}

func Archive() ([]byte, error) {
	metadata, err := ReadMetadata()
	if err != nil {
		return nil, err
	}
	if len(imageArchive) == 0 {
		return nil, errors.New("embedded Nano workload image is not available")
	}
	actualArchiveDigest := fmt.Sprintf("sha256:%x", sha256.Sum256(imageArchive))
	if metadata.ArchiveSHA256 != actualArchiveDigest {
		return nil, fmt.Errorf(
			"embedded Nano image archive checksum mismatch: expected %s, got %s",
			metadata.ArchiveSHA256,
			actualArchiveDigest,
		)
	}

	return imageArchive, nil
}

func Embedded() ([]byte, Metadata, error) {
	metadata, err := ReadMetadata()
	if err != nil {
		return nil, Metadata{}, err
	}
	archive, err := Archive()
	if err != nil {
		return nil, Metadata{}, err
	}

	return archive, metadata, nil
}
