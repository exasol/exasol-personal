// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	localassets "github.com/exasol/exasol-personal/internal/localruntime/assets"
)

func main() {
	var (
		version      = flag.String("version", "", "embedded payload version")
		architecture = flag.String("architecture", "", "embedded payload architecture")
		runPath      = flag.String("run", "", "path to the Linux .run payload")
		kernelPath   = flag.String("kernel", "", "path to the guest kernel")
		initrdPath   = flag.String("initrd", "", "path to the guest initrd")
		outputDir    = flag.String("out-dir", "", "output directory for metadata.json and payload.tar.gz")
	)
	flag.Parse()

	for _, required := range []struct {
		name  string
		value string
	}{
		{"version", *version},
		{"architecture", *architecture},
		{"run", *runPath},
		{"kernel", *kernelPath},
		{"initrd", *initrdPath},
		{"out-dir", *outputDir},
	} {
		if required.value == "" {
			exitf("missing required flag: -%s", required.name)
		}
	}

	if err := os.MkdirAll(*outputDir, 0o700); err != nil {
		exitf("failed to create output directory: %v", err)
	}

	metadata, err := buildMetadata(*version, *architecture, *runPath, *kernelPath, *initrdPath)
	if err != nil {
		exitf("failed to build embedded payload metadata: %v", err)
	}

	metadataJSON, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		exitf("failed to encode metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(*outputDir, "metadata.json"), append(metadataJSON, '\n'), 0o600); err != nil {
		exitf("failed to write metadata.json: %v", err)
	}

	if err := writeBundleArchive(filepath.Join(*outputDir, "payload.tar.gz"), map[string]string{
		"run":    *runPath,
		"kernel": *kernelPath,
		"initrd": *initrdPath,
	}); err != nil {
		exitf("failed to write payload bundle: %v", err)
	}
}

func buildMetadata(
	version string,
	architecture string,
	runPath string,
	kernelPath string,
	initrdPath string,
) (*localassets.EmbeddedMetadata, error) {
	runChecksum, err := fileSHA256(runPath)
	if err != nil {
		return nil, err
	}
	kernelChecksum, err := fileSHA256(kernelPath)
	if err != nil {
		return nil, err
	}
	initrdChecksum, err := fileSHA256(initrdPath)
	if err != nil {
		return nil, err
	}

	return &localassets.EmbeddedMetadata{
		Version:      version,
		Architecture: architecture,
		Run: &localassets.Asset{
			Filename: filepath.Base(runPath),
			SHA256:   runChecksum,
		},
		Boot: &localassets.BootAssets{
			Kernel: &localassets.Asset{
				Filename: filepath.Base(kernelPath),
				SHA256:   kernelChecksum,
			},
			Initrd: &localassets.Asset{
				Filename: filepath.Base(initrdPath),
				SHA256:   initrdChecksum,
			},
		},
	}, nil
}

func writeBundleArchive(path string, files map[string]string) error {
	output, err := os.Create(path)
	if err != nil {
		return err
	}
	defer output.Close()

	gzipWriter := gzip.NewWriter(output)
	defer gzipWriter.Close()

	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	for role, sourcePath := range files {
		if err := addFileToArchive(tarWriter, role, sourcePath); err != nil {
			return err
		}
	}

	return nil
}

func addFileToArchive(writer *tar.Writer, role string, sourcePath string) error {
	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	info, err := sourceFile.Stat()
	if err != nil {
		return err
	}

	header := &tar.Header{
		Name: filepath.Join("bundle", filepath.Base(sourcePath)),
		Mode: int64(info.Mode().Perm()),
		Size: info.Size(),
	}
	if role == "run" {
		header.Mode = 0o700
	}

	if err := writer.WriteHeader(header); err != nil {
		return err
	}
	_, err = io.Copy(writer, sourceFile)
	return err
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func exitf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
