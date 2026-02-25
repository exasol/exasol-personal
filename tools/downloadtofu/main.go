// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
)

const tofuVersion = "1.10.5"

func tofuCompressedFileName(operatingSystem, arch string) string {
	return fmt.Sprintf("tofu_%s_%s_%s.tar.gz", tofuVersion, operatingSystem, arch)
}

func tofuUrl(operatingSystem, arch string) string {
	return fmt.Sprintf("https://github.com/opentofu/opentofu/releases/download/v%s/%s",
		tofuVersion,
		tofuCompressedFileName(operatingSystem, arch),
	)
}

// Download open tofu and unzip it into the specified dir.
// If the dir already contains the required binary (accounting for os and arch), do nothing.
func main() {
	err := downloadTofu()
	if err != nil {
		log.Fatal(err)
	}

	err = generateThirdPartyLicenses()
	if err != nil {
		log.Fatal(err)
	}
}

// nolint: gosec
func downloadTofu() error {
	goos := flag.String("goos", "", "operating system")
	goarch := flag.String("goarch", "", "arch")
	destDir := flag.String("dir", ".", "Enable debug mode")

	flag.Parse()

	if *goos == "" {
		*goos = runtime.GOOS
	}

	if *goarch == "" {
		*goarch = runtime.GOARCH
	}

	*destDir = filepath.Join(*destDir, *goos, *goarch)

	compressedFilePath := path.Join(*destDir, tofuCompressedFileName(*goos, *goarch))

	if _, err := os.Stat(compressedFilePath); os.IsNotExist(err) {
		if err := downloadTofuBin(destDir, goos, goarch, compressedFilePath); err != nil {
			return err
		}
	} else {
		fmt.Println("open tofu is already download") // nolint: revive,forbidigo
	}

	compressedFile, err := os.Open(compressedFilePath)
	if err != nil {
		return fmt.Errorf("failed to open compressed file: %w", err)
	}

	fmt.Printf("uncompressing tofu release %s\n", compressedFilePath) // nolint: revive,forbidigo

	uncompressedStream, err := gzip.NewReader(compressedFile)
	if err != nil {
		return fmt.Errorf("gzip reader error: %w", err)
	}
	defer uncompressedStream.Close()

	tarReader := tar.NewReader(uncompressedStream)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}

		if err != nil {
			return fmt.Errorf("tar reader error: %w", err)
		}

		target := filepath.Join(*destDir, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("mkdir error: %w", err)
			}
		case tar.TypeReg:
			// nolint: mnd
			if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
				return fmt.Errorf("mkdir parent error: %w", err)
			}

			outFile, err := os.Create(target)
			if err != nil {
				return fmt.Errorf("create file error: %w", err)
			}

			if _, err := io.Copy(outFile, tarReader); err != nil {
				outFile.Close() // nolint: revive
				return fmt.Errorf("copy file error: %w", err)
			}

			err = outFile.Close()
			if err != nil {
				return err
			}
		default:
			// nolint: revive,forbidigo
			fmt.Printf("Skipping unknown type: %c in %s\n", header.Typeflag, header.Name)
		}
	}

	return nil
}

func downloadTofuBin(destDir, goos, goarch *string, compressedFilePath string) error {
	url := tofuUrl(*goos, *goarch)
	fmt.Printf("dowloading open tofu release %s\n", url) // nolint: revive,forbidigo

	// Remove the output dir if it already exists. Otherwise it may cause problems if
	// it contains files from a previous cross-compile.
	err := os.RemoveAll(*destDir)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	err = os.MkdirAll(*destDir, 0o700) // nolint: mnd
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close() // nolint: revive

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status: %s", resp.Status)
	}

	compressedFile, err := os.Create(compressedFilePath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}

	_, err = io.Copy(compressedFile, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write to file: %w", err)
	}

	return compressedFile.Close()
}

func generateThirdPartyLicenses() error {
	content := fmt.Sprintf(`Third-Party Software Licenses
==============================

This software includes or distributes the following third-party software:

## OpenTofu

**Version:** %s
**License:** Mozilla Public License 2.0 (MPL-2.0)
**Source Code:** https://github.com/opentofu/opentofu/tree/v%s
**Copyright:** Copyright (c) 2023 The OpenTofu Authors

### Description

This software embeds the OpenTofu binary, an open-source infrastructure as code tool.
OpenTofu is distributed under the Mozilla Public License 2.0.

### License Text

The full text of the Mozilla Public License 2.0 can be found at:
https://www.mozilla.org/en-US/MPL/2.0/

### Source Code Availability

The complete source code for OpenTofu version %s is available at:
- GitHub Repository: https://github.com/opentofu/opentofu
- Specific Version: https://github.com/opentofu/opentofu/tree/v%s
- Release Archive: https://github.com/opentofu/opentofu/releases/tag/v%s

### Mozilla Public License 2.0 Summary

The Mozilla Public License 2.0 is a weak copyleft license that allows you to:
- Use the software for any purpose
- Modify the software
- Distribute the software

Under the following conditions:
- The source code of the MPL-licensed portions must be made available under MPL-2.0
- Modifications to MPL-licensed files must also be released under MPL-2.0
- You must include a notice about the use of MPL-licensed software

---

Full MPL-2.0 License Text:
https://www.mozilla.org/media/MPL/2.0/index.815ca599c9df.txt
`, tofuVersion, tofuVersion, tofuVersion, tofuVersion, tofuVersion)

	// Write to repository root (assuming we're run from repo root or with proper working directory)
	err := os.WriteFile("THIRD_PARTY_LICENSES", []byte(content), 0o644) // nolint: mnd,gosec
	if err != nil {
		return fmt.Errorf("failed to write THIRD_PARTY_LICENSES: %w", err)
	}

	fmt.Println("Generated THIRD_PARTY_LICENSES file") // nolint: revive,forbidigo

	return nil
}
