// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package runtimeartifacts

import (
	"archive/tar"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/opencontainers/go-digest"
	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
)

type ociArchiveFixture struct {
	imageName         string
	indexMediaType    string
	extraIndexEntries int
	corruptManifest   bool
}

func TestDockerArchiveManifest_DescribesTheIndexedImage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		imageName    string
		wantRepoTags []string
	}{
		{"docker.io/exasol/nano:2026.2.0-nano.6", []string{"exasol/nano:2026.2.0-nano.6"}},
		{"docker.io/library/busybox:1.37", []string{"busybox:1.37"}},
		{"registry.example.org:5000/foo:1.5", []string{"registry.example.org:5000/foo:1.5"}},
		{"example.org/foo", nil},
	}

	for _, testCase := range tests {
		t.Run(testCase.imageName, func(t *testing.T) {
			t.Parallel()

			// Given
			archivePath, blobs := writeOCIArchiveFixture(
				t, ociArchiveFixture{imageName: testCase.imageName},
			)

			// When
			data, err := dockerArchiveManifest(archivePath)
			// Then
			if err != nil {
				t.Fatalf("describe OCI archive: %v", err)
			}
			var entries []dockerArchiveManifestEntry
			if err := json.Unmarshal(data, &entries); err != nil {
				t.Fatalf("parse manifest.json: %v", err)
			}
			want := []dockerArchiveManifestEntry{{
				Config:   blobs.config,
				RepoTags: testCase.wantRepoTags,
				Layers:   blobs.layers,
			}}
			if !reflect.DeepEqual(entries, want) {
				t.Fatalf("manifest.json = %+v, want %+v", entries, want)
			}
			for _, blob := range append([]string{entries[0].Config}, entries[0].Layers...) {
				if _, err := readTarFile(archivePath, blob); err != nil {
					t.Errorf("manifest.json refers to %s: %v", blob, err)
				}
			}
		})
	}
}

func TestDockerArchiveManifest_RejectsArchivesItCannotDescribe(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		fixture ociArchiveFixture
		wantErr string
	}{
		"several images": {
			ociArchiveFixture{imageName: "example.org/foo:1", extraIndexEntries: 1},
			"exactly one image, found 2",
		},
		"nested index": {
			ociArchiveFixture{
				imageName: "example.org/foo:1", indexMediaType: imgspecv1.MediaTypeImageIndex,
			},
			"unsupported OCI index entry media type",
		},
		"manifest not matching its digest": {
			ociArchiveFixture{imageName: "example.org/foo:1", corruptManifest: true},
			"image manifest does not match digest",
		},
	}

	for name, testCase := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// Given
			archivePath, _ := writeOCIArchiveFixture(t, testCase.fixture)

			// When
			_, err := dockerArchiveManifest(archivePath)

			// Then
			if err == nil || !strings.Contains(err.Error(), testCase.wantErr) {
				t.Fatalf("error = %v, want one containing %q", err, testCase.wantErr)
			}
		})
	}
}

type ociImageBlobs struct {
	config string
	layers []string
}

// writeOCIArchiveFixture writes an OCI archive of one image with two layers and
// returns its path and the blob paths of the image's config and layers.
func writeOCIArchiveFixture(t *testing.T, fixture ociArchiveFixture) (string, ociImageBlobs) {
	t.Helper()

	files := map[string][]byte{}
	blobPathOf := func(descriptor imgspecv1.Descriptor) string {
		return "blobs/sha256/" + descriptor.Digest.Encoded()
	}
	addBlob := func(data []byte) imgspecv1.Descriptor {
		descriptor := imgspecv1.Descriptor{Digest: digest.FromBytes(data), Size: int64(len(data))}
		files[blobPathOf(descriptor)] = data

		return descriptor
	}
	config := addBlob([]byte(`{"architecture":"amd64","os":"linux"}`))
	config.MediaType = imgspecv1.MediaTypeImageConfig
	layers := []imgspecv1.Descriptor{addBlob([]byte("layer one")), addBlob([]byte("layer two"))}
	for index := range layers {
		layers[index].MediaType = imgspecv1.MediaTypeImageLayerGzip
	}
	manifestData, err := json.Marshal(imgspecv1.Manifest{
		MediaType: imgspecv1.MediaTypeImageManifest,
		Config:    config,
		Layers:    layers,
	})
	if err != nil {
		t.Fatalf("encode image manifest: %v", err)
	}
	manifestDescriptor := addBlob(manifestData)
	if fixture.corruptManifest {
		manifestPath := blobPathOf(manifestDescriptor)
		files[manifestPath] = append(files[manifestPath], ' ')
	}
	manifestDescriptor.MediaType = imgspecv1.MediaTypeImageManifest
	if fixture.indexMediaType != "" {
		manifestDescriptor.MediaType = fixture.indexMediaType
	}
	if fixture.imageName != "" {
		manifestDescriptor.Annotations = map[string]string{
			imgspecv1.AnnotationRefName: fixture.imageName,
		}
	}
	index := imgspecv1.Index{SchemaVersion: 2, MediaType: imgspecv1.MediaTypeImageIndex}
	for range fixture.extraIndexEntries + 1 {
		index.Manifests = append(index.Manifests, manifestDescriptor)
	}
	indexData, err := json.Marshal(index)
	if err != nil {
		t.Fatalf("encode OCI index: %v", err)
	}
	files[imgspecv1.ImageIndexFile] = indexData
	files[imgspecv1.ImageLayoutFile] = []byte(`{"imageLayoutVersion":"1.0.0"}`)

	archivePath := filepath.Join(t.TempDir(), "image.tar")
	writeTarFiles(t, archivePath, files)

	return archivePath, ociImageBlobs{
		config: blobPathOf(config),
		layers: []string{blobPathOf(layers[0]), blobPathOf(layers[1])},
	}
}

func writeTarFiles(t *testing.T, archivePath string, files map[string][]byte) {
	t.Helper()

	archive, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create tar archive: %v", err)
	}
	writer := tar.NewWriter(archive)
	for name, data := range files {
		header := &tar.Header{
			Typeflag: tar.TypeReg, Name: name, Mode: 0o644, Size: int64(len(data)),
		}
		if err := writer.WriteHeader(header); err != nil {
			t.Fatalf("write tar header %s: %v", name, err)
		}
		if _, err := writer.Write(data); err != nil {
			t.Fatalf("write tar entry %s: %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	if err := archive.Close(); err != nil {
		t.Fatalf("close tar archive: %v", err)
	}
}
