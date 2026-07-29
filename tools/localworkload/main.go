// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/exasol/exasol-personal/assets/localworkloadbin"
)

const placeholderDigest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("expected subcommand: placeholder, update, generate, fetch, or stage")
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	goos := flags.String("goos", "", "target operating system")
	goarch := flags.String("goarch", "", "target architecture")
	archive := flags.String(
		"archive",
		strings.TrimSpace(os.Getenv("NANO_IMAGE_ARCHIVE")),
		"Nano podman-save archive",
	)
	reference := flags.String(
		"reference",
		strings.TrimSpace(os.Getenv("NANO_IMAGE_REFERENCE")),
		"digest-pinned Nano reference",
	)
	root := flags.String("root", "assets/localworkloadbin/generated", "generated artifact root")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}

	// FIXME maybe add tests for these
	if args[0] == "update" {
		return updateLocalImagePins()
	}

	if args[0] == "generate" {
		return generate()
	}

	var targets []ociPlatform
	targets = nil
	if *goos != "" || *goarch != "" {
		targets = []ociPlatform{{OS: *goos, Architecture: *goarch}}
	}
	pins, err := localImagePins(targets)
	if err != nil {
		return err
	}
	for pinName, pin := range pins {
		if args[0] == "placeholder" {
			if err := writePlaceholder(*root, pin); err != nil {
				return err
			}
			continue
		}
		if args[0] == "fetch" {
			if err := fetchAndStage(*root, pin); err != nil {
				return err
			}
			continue
		}
		if args[0] != "stage" {
			return fmt.Errorf("unknown subcommand %q", args[0])
		}
		archiveInput := platformInput(
			*archive,
			"NANO_IMAGE_ARCHIVE",
			pinName,
		)
		referenceInput := platformInput(
			*reference,
			"NANO_IMAGE_REFERENCE",
			pinName,
		)
		if archiveInput == "" || referenceInput == "" {
			continue
		}
		if err := stage(
			*root,
			pin,
			archiveInput,
			referenceInput,
		); err != nil {
			return err
		}
	}
	return nil
}

func platformInput(
	fallback string,
	name, pinName string,
) string {
	suffix := strings.ToUpper(strings.ReplaceAll(pinName, "-", "_"))
	if value := strings.TrimSpace(os.Getenv(name + "_" + suffix)); value != "" {
		return value
	}
	return strings.TrimSpace(fallback)
}

func writePlaceholder(root string, pin nanoImagePin) error {
	directory := filepath.Join(root, pin.ContainerPlatform.OS, pin.ContainerPlatform.Architecture)
	archivePath := filepath.Join(directory, "exasol-nano.tar.gz")
	metadataPath := filepath.Join(directory, "image.json")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	if _, err := os.Stat(archivePath); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(archivePath, []byte("development placeholder"), 0o600); err != nil {
			return err
		}
	}
	if _, err := os.Stat(metadataPath); errors.Is(err, os.ErrNotExist) {
		archiveData, err := os.ReadFile(archivePath)
		if err != nil {
			return err
		}
		metadata := localworkloadbin.Metadata{
			ImageReference: "docker.io/exasol/nano@" + placeholderDigest,
			ImageDigest:    placeholderDigest,
			ArchiveSHA256:  fmt.Sprintf("sha256:%x", sha256.Sum256(archiveData)),
			Platform:       pin.ContainerPlatform.OS + "/" + pin.ContainerPlatform.Architecture,
		}
		data, err := json.Marshal(metadata)
		if err != nil {
			return err
		}
		return os.WriteFile(metadataPath, data, 0o600)
	}
	return nil
}

func stage(root string, pin nanoImagePin, archive, reference string) error {
	marker := "@sha256:"
	index := strings.LastIndex(reference, marker)
	if index < 0 || len(reference[index+len(marker):]) != 64 {
		return errors.New("NANO_IMAGE_REFERENCE must be pinned by a 64-character sha256 digest")
	}
	info, err := os.Stat(archive)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return errors.New("Nano image archive must be a non-empty regular file")
	}
	digest := reference[index+1:]
	if err := validateOCIArchive(
		archive,
		digest,
		pin.ContainerPlatform,
	); err != nil {
		return err
	}
	directory := filepath.Join(root, pin.ContainerPlatform.OS, pin.ContainerPlatform.Architecture)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	input, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(
		filepath.Join(directory, "exasol-nano.tar.gz"),
		os.O_CREATE|os.O_TRUNC|os.O_WRONLY,
		0o600,
	)
	if err != nil {
		return err
	}
	archiveHash := sha256.New()
	if _, err := io.Copy(io.MultiWriter(output, archiveHash), input); err != nil {
		output.Close()
		return err
	}
	if err := output.Close(); err != nil {
		return err
	}
	metadata := localworkloadbin.Metadata{
		ImageReference: reference,
		ImageDigest:    digest,
		ArchiveSHA256:  "sha256:" + fmt.Sprintf("%x", archiveHash.Sum(nil)),
		Platform:       pin.ContainerPlatform.OS + "/" + pin.ContainerPlatform.Architecture,
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(directory, "image.json"), data, 0o600)
}

func fetchAndStage(root string, pin nanoImagePin) error {
	temporary, err := os.MkdirTemp("", "exasol-personal-nano-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporary)
	ociArchive := filepath.Join(temporary, "nano.oci.tar")
	if err := exportOCIArchive(pin, ociArchive); err != nil {
		return fmt.Errorf(
			"failed to fetch pinned Nano image for %s/%s: %w",
			pin.ContainerPlatform.OS,
			pin.ContainerPlatform.Architecture,
			err,
		)
	}
	compressedArchive := filepath.Join(temporary, "nano.oci.tar.gz")
	if err := gzipFile(ociArchive, compressedArchive); err != nil {
		return err
	}

	return stage(root, pin, compressedArchive, pin.Reference)
}

func exportOCIArchive(pin nanoImagePin, archivePath string) error {
	if _, err := exec.LookPath("skopeo"); err == nil {
		cmd := exec.Command(
			"skopeo",
			"copy",
			"--preserve-digests",
			"--override-os", pin.ContainerPlatform.OS,
			"--override-arch", pin.ContainerPlatform.Architecture,
			"docker://"+pin.Reference,
			// Specify the reference again as the target tag so it's part of the image
			// and will be loaded by podman.
			"oci-archive:"+archivePath+":"+pin.Reference,
		)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	return errors.New("skopeo is required to fetch the pinned Nano image")
}

func gzipFile(sourcePath, targetPath string) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()
	target, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	compressed := gzip.NewWriter(target)
	if _, err := io.Copy(compressed, source); err != nil {
		compressed.Close()
		target.Close()
		return err
	}
	if err := compressed.Close(); err != nil {
		target.Close()
		return err
	}

	return target.Close()
}
