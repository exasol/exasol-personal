// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package runtimeartifacts

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"go.podman.io/image/v5/transports"
)

func TestParseDockerImageReference(t *testing.T) {
	t.Parallel()

	const digest = "sha256:93512673ca38053cb45fa33eaa9ac999fc93c2c8f70c873d054432433c5e81bf"

	tests := []struct {
		url             string
		wantSource      string
		wantDestination string
	}{
		{
			"docker://example.org/foo",
			"docker://example.org/foo:latest",
			"example.org/foo",
		},
		{
			"docker://example.org/foo:1.5",
			"docker://example.org/foo:1.5",
			"example.org/foo:1.5",
		},
		{
			"docker://registry.example.org:5000/foo:latest",
			"docker://registry.example.org:5000/foo:latest",
			"registry.example.org:5000/foo:latest",
		},
		{
			"docker://example.org/foo@" + digest,
			"docker://example.org/foo@" + digest,
			"example.org/foo",
		},
		{
			"docker://example.org/foo:1.5@" + digest,
			"docker://example.org/foo@" + digest,
			"example.org/foo:1.5",
		},
		{
			"docker://exasol/nano:1.5@" + digest,
			"docker://exasol/nano@" + digest,
			"docker.io/exasol/nano:1.5",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.url, func(t *testing.T) {
			t.Parallel()

			// Given
			source, err := parseImageReference(testCase.url)
			if err != nil {
				t.Fatalf("parse image reference: %v", err)
			}

			// When
			gotSource := transports.ImageName(source.source)

			// Then
			if gotSource != testCase.wantSource {
				t.Errorf(
					"source reference for %q = %q, want %q",
					testCase.url,
					gotSource,
					testCase.wantSource,
				)
			}
			if source.destinationImage != testCase.wantDestination {
				t.Errorf(
					"destination reference for %q = %q, want %q",
					testCase.url,
					source.destinationImage,
					testCase.wantDestination,
				)
			}
		})
	}
}

func TestParseImageReference_OCIUsesDescriptorAnnotation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		descriptorImage string
		sourceImage     string
		wantErr         bool
	}{
		{
			name:            "explicitly selected image",
			descriptorImage: "example.org/foo:latest",
			sourceImage:     "example.org/foo:latest",
		},
		{name: "implicitly selected image", descriptorImage: "example.org/foo:latest"},
		{name: "unnamed image", wantErr: true},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			// Given
			ociPath := createOCIIndex(t, testCase.descriptorImage)
			sourceName := "oci:" + ociPath
			if testCase.sourceImage != "" {
				sourceName += ":" + testCase.sourceImage
			}
			// When
			parsedReference, err := parseImageReference(sourceName)
			// Then
			if testCase.wantErr {
				if err == nil {
					t.Fatal("expected parsing OCI reference to fail")
				}

				return
			}
			if err != nil {
				t.Fatalf("parse OCI reference: %v", err)
			}

			got := parsedReference.destinationImage
			if got != testCase.descriptorImage {
				t.Errorf("archive image name = %q, want %q", got, testCase.descriptorImage)
			}
		})
	}
}

func TestDockerSource_CanFetch(t *testing.T) {
	t.Parallel()

	const digest = "sha256:93512673ca38053cb45fa33eaa9ac999fc93c2c8f70c873d054432433c5e81bf"

	src := &DockerSource{}
	ociPath := filepath.Join(t.TempDir(), "image")
	trueURLs := []string{
		"docker://example.org/foo",
		"docker://example.org/foo:latest",
		"docker://example.org/foo:1.5",
		"docker://registry.example.org:5000/foo:latest",
		"docker://example.org/foo@" + digest,
		"docker://example.org/foo:latest@" + digest,
		"docker://example.org/foo:1.5@" + digest,
		"oci:" + ociPath,
		"oci:" + ociPath + ":latest",
		"oci-archive:" + ociPath,
		"oci-archive:" + ociPath + ":latest",
	}
	for _, url := range trueURLs {
		if !src.CanFetch(url) {
			t.Errorf("CanFetch(%q) = false, want true", url)
		}
	}

	falseURLs := []string{
		"https://example.com/archive.tar.gz",
		"http://example.com/archive.tar.gz",
		"file:///path/to/dir",
		"",
	}
	for _, url := range falseURLs {
		if src.CanFetch(url) {
			t.Errorf("CanFetch(%q) = true, want false", url)
		}
	}
}

func TestDockerSource_Fetch_LocalOCIImages(t *testing.T) {
	t.Parallel()

	requireContainerTools(t)

	// Given
	image := createTestContainerImage(t)
	src := &DockerSource{}
	//nolint:usetesting // The relative path becomes the OCI archive image annotation.
	fixtureDir, err := os.MkdirTemp(".", "docker-source-test-")
	if err != nil {
		t.Fatalf("create OCI fixture directory: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(fixtureDir)
	})
	fixtureDir = filepath.Base(fixtureDir)
	cases := []struct {
		name       string
		saveFormat string
		urlScheme  string
		sourcePath string
	}{
		{
			name:       "oci archive",
			saveFormat: "oci-archive",
			urlScheme:  "oci-archive:",
			sourcePath: filepath.Join(fixtureDir, "image.tar"),
		},
		{
			name:       "oci directory",
			saveFormat: "oci-dir",
			urlScheme:  "oci:",
			sourcePath: filepath.Join(fixtureDir, "image"),
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			// Given
			runPodman(
				t, "save", "--format", testCase.saveFormat, "--output", testCase.sourcePath, image,
			)
			url := testCase.urlScheme + testCase.sourcePath
			parsedReference, err := parseImageReference(url)
			if err != nil {
				t.Fatalf("parse Docker source URL: %v", err)
			}
			dstPath := filepath.Join(t.TempDir(), "download.tar")

			// When
			if _, err := src.Fetch(context.Background(), url, dstPath); err != nil {
				t.Fatalf("fetch %s: %v", testCase.name, err)
			}
			secondDstPath := filepath.Join(t.TempDir(), "download.tar")
			if _, err := src.Fetch(context.Background(), url, secondDstPath); err != nil {
				t.Fatalf("fetch %s a second time: %v", testCase.name, err)
			}
			runPodman(t, "load", "--input", dstPath)

			// Then
			firstArchive, err := os.ReadFile(dstPath)
			if err != nil {
				t.Fatalf("read first archive: %v", err)
			}
			secondArchive, err := os.ReadFile(secondDstPath)
			if err != nil {
				t.Fatalf("read second archive: %v", err)
			}
			if !bytes.Equal(firstArchive, secondArchive) {
				t.Fatal("expected deterministic archive output")
			}
			output := runPodman(
				t,
				"image",
				"inspect",
				"--format",
				"{{ index .Config.Labels \"runtimeartifacts.test\" }}",
				parsedReference.destinationImage,
			)
			if output != "local-oci-image\n" {
				t.Fatalf("expected loaded image label, got %q", output)
			}
		})
	}
}

func createOCIIndex(t *testing.T, imageName string) string {
	t.Helper()

	annotations := ""
	if imageName != "" {
		annotations = fmt.Sprintf(
			`,"annotations":{"org.opencontainers.image.ref.name":%q}`,
			imageName,
		)
	}
	index := fmt.Sprintf(
		`{"schemaVersion":2,"manifests":[{`+
			`"mediaType":"application/vnd.oci.image.manifest.v1+json",`+
			`"digest":"sha256:93512673ca38053cb45fa33eaa9ac999fc93c2c8f70c873d054432433c5e81bf",`+
			`"size":0%s}]}`,
		annotations,
	)
	ociPath := t.TempDir()
	indexPath := filepath.Join(ociPath, "index.json")
	if err := os.WriteFile(indexPath, []byte(index), filePerm); err != nil {
		t.Fatalf("write OCI index: %v", err)
	}

	return ociPath
}

func requireContainerTools(t *testing.T) {
	t.Helper()

	if _, err := exec.LookPath("podman"); err != nil {
		t.Skipf("podman is required for container image tests: %v", err)
	}
	podmanInfo := exec.CommandContext(t.Context(), "podman", "info")
	if output, err := podmanInfo.CombinedOutput(); err != nil {
		t.Skipf("podman must be usable for container image tests: %v\n%s", err, output)
	}
}

func createTestContainerImage(t *testing.T) string {
	t.Helper()

	contextDir := t.TempDir()
	containerfile := filepath.Join(contextDir, "Containerfile")
	containerfileContents := []byte("FROM scratch\nLABEL runtimeartifacts.test=local-oci-image\n")
	if err := os.WriteFile(containerfile, containerfileContents, filePerm); err != nil {
		t.Fatalf("write Containerfile: %v", err)
	}

	image := fmt.Sprintf("runtimeartifacts-test-%d", time.Now().UnixNano())
	runPodman(t, "build", "--tag", image, contextDir)
	t.Cleanup(func() {
		removePodmanImage(t, image)
	})

	return image
}

func removePodmanImage(t *testing.T, image string) {
	t.Helper()

	cmd := exec.CommandContext(context.Background(), "podman", "image", "rm", "--force", image)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("remove test image %q: %v\n%s", image, err, output)
	}
}

func runPodman(t *testing.T, args ...string) string {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "podman", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("podman %v: %v\n%s", args, err, output)
	}

	return string(output)
}
