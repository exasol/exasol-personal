// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
)

const maxOCIJSONSize = 256 * 1024 * 1024

type ociDescriptor struct {
	Digest   string       `json:"digest"`
	Platform *ociPlatform `json:"platform,omitempty"`
}

type ociPlatform struct {
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
}

type ociIndex struct {
	SchemaVersion int             `json:"schemaVersion"`
	Manifests     []ociDescriptor `json:"manifests"`
}

type ociManifest struct {
	SchemaVersion int             `json:"schemaVersion"`
	Config        ociDescriptor   `json:"config"`
	Layers        []ociDescriptor `json:"layers"`
}

type ociConfig struct {
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
	Config       struct {
		Entrypoint []string `json:"Entrypoint"`
	} `json:"config"`
}

type ociArchiveEntry struct {
	Digest string
	Data   []byte
}

func validateOCIArchive(
	archivePath, expectedDigest string, expectedPlatform ociPlatform,
) error {
	entries, layout, indexData, err := scanOCIArchive(archivePath)
	if err != nil {
		return err
	}
	if len(layout) == 0 {
		return errors.New("Nano image archive is not an OCI image layout: oci-layout is missing")
	}
	var layoutVersion struct {
		ImageLayoutVersion string `json:"imageLayoutVersion"`
	}
	if err := json.Unmarshal(layout, &layoutVersion); err != nil ||
		layoutVersion.ImageLayoutVersion != "1.0.0" {
		return errors.New("Nano image archive has an unsupported oci-layout")
	}
	var index ociIndex
	if err := json.Unmarshal(indexData, &index); err != nil || index.SchemaVersion != 2 {
		return errors.New("Nano image archive has an invalid OCI index")
	}
	var selected ociDescriptor
	for _, descriptor := range index.Manifests {
		if descriptor.Platform != nil &&
			*descriptor.Platform == expectedPlatform {
			selected = descriptor
			break
		}
	}
	if selected.Digest == "" {
		for _, descriptor := range index.Manifests {
			config, configErr := readOCIConfig(entries, descriptor)
			if configErr != nil {
				return configErr
			}
			if config.OS == expectedPlatform.OS &&
				config.Architecture == expectedPlatform.Architecture {
				selected = descriptor
				break
			}
		}
	}
	if selected.Digest == "" {
		return fmt.Errorf(
			"Nano image archive has no manifest for %s/%s",
			expectedPlatform.OS,
			expectedPlatform.Architecture,
		)
	}
	if selected.Digest != expectedDigest {
		return fmt.Errorf(
			"Nano image reference digest %s does not match OCI manifest %s",
			expectedDigest,
			selected.Digest,
		)
	}
	manifestData, err := requireOCIEntry(entries, selected.Digest)
	if err != nil {
		return err
	}
	var manifest ociManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil ||
		manifest.SchemaVersion != 2 {
		return errors.New("Nano image archive has an invalid OCI manifest")
	}
	configData, err := requireOCIEntry(entries, manifest.Config.Digest)
	if err != nil {
		return err
	}
	var config ociConfig
	if err := json.Unmarshal(configData, &config); err != nil {
		return errors.New("Nano image archive has an invalid OCI image config")
	}
	if config.OS != expectedPlatform.OS || config.Architecture != expectedPlatform.Architecture {
		return fmt.Errorf(
			"Nano image config targets %s/%s, expected %s/%s",
			config.OS,
			config.Architecture,
			expectedPlatform.OS,
			expectedPlatform.Architecture,
		)
	}
	for _, layer := range manifest.Layers {
		if _, err := requireOCIEntry(entries, layer.Digest); err != nil {
			return err
		}
	}

	return nil
}

func readOCIConfig(
	entries map[string]ociArchiveEntry,
	descriptor ociDescriptor,
) (ociConfig, error) {
	manifestData, err := requireOCIEntry(entries, descriptor.Digest)
	if err != nil {
		return ociConfig{}, err
	}
	var manifest ociManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil ||
		manifest.SchemaVersion != 2 {
		return ociConfig{}, errors.New("Nano image archive has an invalid OCI manifest")
	}
	configData, err := requireOCIEntry(entries, manifest.Config.Digest)
	if err != nil {
		return ociConfig{}, err
	}
	var config ociConfig
	if err := json.Unmarshal(configData, &config); err != nil {
		return ociConfig{}, errors.New("Nano image archive has an invalid OCI image config")
	}

	return config, nil
}

func scanOCIArchive(
	archivePath string,
) (map[string]ociArchiveEntry, []byte, []byte, error) {
	file, err := os.Open(archivePath)
	if err != nil {
		return nil, nil, nil, err
	}
	defer file.Close()
	compressed, err := gzip.NewReader(file)
	if err != nil {
		return nil, nil, nil, errors.New("Nano image archive must be a gzip-compressed OCI layout")
	}
	defer compressed.Close()

	entries := map[string]ociArchiveEntry{}
	var layout, index []byte
	reader := tar.NewReader(compressed)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to read OCI image archive: %w", err)
		}
		name := path.Clean(strings.TrimPrefix(header.Name, "./"))
		if header.Typeflag != tar.TypeReg {
			continue
		}
		hash := sha256.New()
		var capture []byte
		if header.Size <= maxOCIJSONSize {
			capture = make([]byte, header.Size)
			if _, err := io.ReadFull(io.TeeReader(reader, hash), capture); err != nil {
				return nil, nil, nil, err
			}
		} else if _, err := io.Copy(hash, reader); err != nil {
			return nil, nil, nil, err
		}
		switch name {
		case "oci-layout":
			layout = capture
		case "index.json":
			index = capture
		default:
			if strings.HasPrefix(name, "blobs/sha256/") {
				digest := "sha256:" + strings.TrimPrefix(name, "blobs/sha256/")
				entries[digest] = ociArchiveEntry{
					Digest: "sha256:" + hex.EncodeToString(hash.Sum(nil)),
					Data:   capture,
				}
			}
		}
	}
	if len(index) == 0 {
		return nil, nil, nil, errors.New("Nano image archive is missing index.json")
	}

	return entries, layout, index, nil
}

func requireOCIEntry(entries map[string]ociArchiveEntry, digest string) ([]byte, error) {
	entry, exists := entries[digest]
	if !exists {
		return nil, fmt.Errorf("Nano image archive is missing blob %s", digest)
	}
	if entry.Digest != digest {
		return nil, fmt.Errorf(
			"Nano image archive blob checksum mismatch: expected %s, got %s",
			digest,
			entry.Digest,
		)
	}
	if entry.Data == nil {
		return nil, fmt.Errorf("OCI metadata blob %s exceeds the supported size", digest)
	}

	return entry.Data, nil
}
