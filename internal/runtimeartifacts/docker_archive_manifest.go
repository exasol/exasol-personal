// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package runtimeartifacts

import (
	"archive/tar"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"

	"github.com/opencontainers/go-digest"
	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"go.podman.io/image/v5/docker/reference"
	"go.podman.io/image/v5/manifest"
)

const dockerArchiveManifestName = "manifest.json"

// dockerArchiveManifestEntry is the description of one image in a Docker image
// archive. Docker's classic image store loads only archives that carry it, while
// Podman and Docker's containerd image store read the OCI index.
//
//nolint:tagliatelle // Docker image archives use these exact field names.
type dockerArchiveManifestEntry struct {
	Config   string   `json:"Config"`
	RepoTags []string `json:"RepoTags,omitempty"`
	Layers   []string `json:"Layers"`
}

// dockerArchiveManifest describes the single image of an OCI archive as a Docker
// image archive manifest.json that refers to the archive's own blobs.
func dockerArchiveManifest(archivePath string) ([]byte, error) {
	indexData, err := readTarFile(archivePath, imgspecv1.ImageIndexFile)
	if err != nil {
		return nil, err
	}
	var index imgspecv1.Index
	if err := json.Unmarshal(indexData, &index); err != nil {
		return nil, fmt.Errorf("parse OCI index: %w", err)
	}
	if len(index.Manifests) != 1 {
		return nil, fmt.Errorf(
			"OCI archive must contain exactly one image, found %d", len(index.Manifests),
		)
	}
	descriptor := index.Manifests[0]
	if descriptor.MediaType != imgspecv1.MediaTypeImageManifest &&
		descriptor.MediaType != manifest.DockerV2Schema2MediaType {
		return nil, fmt.Errorf("unsupported OCI index entry media type %q", descriptor.MediaType)
	}

	manifestPath, err := blobPath(descriptor.Digest)
	if err != nil {
		return nil, err
	}
	manifestData, err := readTarFile(archivePath, manifestPath)
	if err != nil {
		return nil, err
	}
	if digest.FromBytes(manifestData) != descriptor.Digest {
		return nil, fmt.Errorf("image manifest does not match digest %s", descriptor.Digest)
	}
	var imageManifest imgspecv1.Manifest
	if err := json.Unmarshal(manifestData, &imageManifest); err != nil {
		return nil, fmt.Errorf("parse image manifest: %w", err)
	}

	configPath, err := blobPath(imageManifest.Config.Digest)
	if err != nil {
		return nil, err
	}
	entry := dockerArchiveManifestEntry{
		Config: configPath,
		Layers: make([]string, 0, len(imageManifest.Layers)),
	}
	for _, layer := range imageManifest.Layers {
		layerPath, err := blobPath(layer.Digest)
		if err != nil {
			return nil, err
		}
		entry.Layers = append(entry.Layers, layerPath)
	}
	if tag := dockerRepoTag(descriptor.Annotations[imgspecv1.AnnotationRefName]); tag != "" {
		entry.RepoTags = []string{tag}
	}

	return json.Marshal([]dockerArchiveManifestEntry{entry})
}

func blobPath(blobDigest digest.Digest) (string, error) {
	if err := blobDigest.Validate(); err != nil {
		return "", fmt.Errorf("invalid blob digest %q: %w", blobDigest, err)
	}

	return path.Join(
		imgspecv1.ImageBlobsDir, blobDigest.Algorithm().String(), blobDigest.Encoded(),
	), nil
}

// dockerRepoTag returns the tag Docker names a loaded image by, or "" for an
// image name without a tag, which Docker loads untagged.
func dockerRepoTag(imageName string) string {
	named, err := reference.ParseNormalizedNamed(imageName)
	if err != nil {
		return ""
	}
	tagged, ok := named.(reference.NamedTagged)
	if !ok {
		return ""
	}
	repoTag, err := reference.WithTag(reference.TrimNamed(named), tagged.Tag())
	if err != nil {
		return ""
	}

	return reference.FamiliarString(repoTag)
}

func readTarFile(archivePath, name string) (_ []byte, retErr error) {
	archive, err := os.Open(archivePath)
	if err != nil {
		return nil, err
	}
	defer func() {
		retErr = errors.Join(retErr, archive.Close())
	}()

	reader := tar.NewReader(archive)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("OCI archive has no %s", name)
		}
		if err != nil {
			return nil, fmt.Errorf("read tar header: %w", err)
		}
		if path.Clean(header.Name) == name && header.Typeflag == tar.TypeReg {
			data, err := io.ReadAll(reader)
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", name, err)
			}

			return data, nil
		}
	}
}
